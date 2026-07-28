package debuglog

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ccLoad/internal/model"
)

const (
	DefaultDir       = "data/debug-logs"
	metadataFileName = "meta.json.gz"
	requestFileName  = "request.body.gz"
	responseFileName = "response.body.gz"
	formatVersion    = 2
	noTokenDirKey    = "_no-token"
)

// FileStore persists Debug logs as <token-key>/<log-id>/*.gz.
type FileStore struct {
	dir   string
	index sync.Map // logs.id -> first-level token directory
}

type metadata struct {
	Version      int    `json:"version"`
	LogID        int64  `json:"log_id"`
	AuthTokenID  int64  `json:"auth_token_id"`
	AuthTokenKey string `json:"auth_token_key"`
	CreatedAt    int64  `json:"created_at"`
	ReqMethod    string `json:"req_method"`
	ReqURL       string `json:"req_url"`
	ReqHeaders   string `json:"req_headers"`
	RespStatus   int    `json:"resp_status"`
	RespHeaders  string `json:"resp_headers"`
}

// EntryInfo is the lightweight file index used by the analyzer.
type EntryInfo struct {
	LogID        int64
	AuthTokenID  int64
	AuthTokenKey string
	CreatedAt    int64
	ModTime      time.Time
}

// CleanupPolicy controls one bounded cleanup pass.
type CleanupPolicy struct {
	Cutoff              time.Time
	MaxDelete           int
	PreserveAuthTokenID int64
	PreserveTokenKey    string
}

type entryPath struct {
	logID    int64
	tokenKey string
	dir      string
	legacy   bool
}

func DirFromEnv() string {
	if dir := strings.TrimSpace(os.Getenv("CCLOAD_DEBUG_LOG_DIR")); dir != "" {
		return dir
	}
	return DefaultDir
}

func NewFileStore(dir string) *FileStore {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = DefaultDir
	}
	return &FileStore{dir: filepath.Clean(dir)}
}

func (s *FileStore) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

// NormalizeTokenKey returns the original API token key for the first-level
// directory. Token keys are intentionally not hashed at the user's request.
func NormalizeTokenKey(value string, tokenID int64) string {
	value = strings.TrimSpace(value)
	if value == noTokenDirKey {
		return value
	}
	if strings.HasPrefix(value, "_token-id-") {
		if id, err := strconv.ParseInt(strings.TrimPrefix(value, "_token-id-"), 10, 64); err == nil && id > 0 {
			return value
		}
	}
	if value != "" {
		return value
	}
	if tokenID > 0 {
		return "_token-id-" + strconv.FormatInt(tokenID, 10)
	}
	return noTokenDirKey
}

func validateTokenDirKey(value string) error {
	if value == noTokenDirKey || strings.HasPrefix(value, "_token-id-") {
		return nil
	}
	if value == "" || len(value) > 255 || value == "." || value == ".." {
		return fmt.Errorf("invalid token key for debug log directory")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '-', '_', '.', '+', '=', '@', ',':
			continue
		default:
			return fmt.Errorf("token key contains unsupported directory character %q", r)
		}
	}
	return nil
}

func (s *FileStore) entryDir(tokenKey string, logID int64) string {
	return filepath.Join(s.dir, tokenKey, strconv.FormatInt(logID, 10))
}

func (s *FileStore) Exists(logID int64) (bool, error) {
	if s == nil {
		return false, errors.New("debug log file store is nil")
	}
	if logID <= 0 {
		return false, fmt.Errorf("invalid debug log id: %d", logID)
	}
	path, err := s.findEntry(logID)
	return path != nil, err
}

// Put compresses files into a temporary directory and publishes the Log ID
// directory with one rename.
func (s *FileStore) Put(ctx context.Context, entry *model.DebugLogEntry) error {
	if s == nil {
		return errors.New("debug log file store is nil")
	}
	if entry == nil {
		return errors.New("debug log entry is nil")
	}
	if entry.LogID <= 0 {
		return fmt.Errorf("invalid debug log id: %d", entry.LogID)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().Unix()
	}
	entry.AuthTokenKey = NormalizeTokenKey(entry.AuthTokenKey, entry.AuthTokenID)
	if err := validateTokenDirKey(entry.AuthTokenKey); err != nil {
		return err
	}

	tokenDir := filepath.Join(s.dir, entry.AuthTokenKey)
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		return fmt.Errorf("create debug log token directory: %w", err)
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return fmt.Errorf("secure debug log directory: %w", err)
	}
	if err := os.Chmod(tokenDir, 0o700); err != nil {
		return fmt.Errorf("secure debug log token directory: %w", err)
	}
	tmpDir, err := os.MkdirTemp(tokenDir, ".tmp-"+strconv.FormatInt(entry.LogID, 10)+"-")
	if err != nil {
		return fmt.Errorf("create debug log temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	if err := os.Chmod(tmpDir, 0o700); err != nil {
		return fmt.Errorf("secure debug log temporary directory: %w", err)
	}

	metaBytes, err := json.Marshal(metadata{
		Version:      formatVersion,
		LogID:        entry.LogID,
		AuthTokenID:  entry.AuthTokenID,
		AuthTokenKey: entry.AuthTokenKey,
		CreatedAt:    entry.CreatedAt,
		ReqMethod:    entry.ReqMethod,
		ReqURL:       entry.ReqURL,
		ReqHeaders:   entry.ReqHeaders,
		RespStatus:   entry.RespStatus,
		RespHeaders:  entry.RespHeaders,
	})
	if err != nil {
		return fmt.Errorf("encode debug log metadata: %w", err)
	}

	files := []struct {
		name string
		data []byte
	}{
		{name: metadataFileName, data: append(metaBytes, '\n')},
		{name: requestFileName, data: entry.ReqBody},
		{name: responseFileName, data: entry.RespBody},
	}
	for _, file := range files {
		if err := writeGzipFile(ctx, filepath.Join(tmpDir, file.name), file.data); err != nil {
			return fmt.Errorf("write compressed debug log %s: %w", file.name, err)
		}
	}

	finalDir := s.entryDir(entry.AuthTokenKey, entry.LogID)
	if err := os.RemoveAll(finalDir); err != nil {
		return fmt.Errorf("replace existing debug log: %w", err)
	}
	if err := os.Rename(tmpDir, finalDir); err != nil {
		return fmt.Errorf("publish debug log: %w", err)
	}
	s.index.Store(entry.LogID, entry.AuthTokenKey)
	return nil
}

func writeGzipFile(ctx context.Context, path string, data []byte) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()

	zw, err := gzip.NewWriterLevel(f, gzip.BestSpeed)
	if err != nil {
		return err
	}
	const chunkSize = 1024 * 1024
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			_ = zw.Close()
			return err
		}
		n := min(len(data), chunkSize)
		written, writeErr := zw.Write(data[:n])
		if writeErr != nil {
			_ = zw.Close()
			return writeErr
		}
		if written != n {
			_ = zw.Close()
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	closed = true
	return nil
}

// Get locates a globally unique logs.id across token directories and expands
// its compressed files only when the detail API or analyzer requests it.
func (s *FileStore) Get(ctx context.Context, logID int64) (*model.DebugLogEntry, error) {
	if s == nil {
		return nil, errors.New("debug log file store is nil")
	}
	if logID <= 0 {
		return nil, fmt.Errorf("invalid debug log id: %d", logID)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.findEntry(logID)
	if err != nil || path == nil {
		return nil, err
	}
	entry, err := s.readEntry(ctx, *path)
	if errors.Is(err, os.ErrNotExist) {
		s.index.Delete(logID)
		return nil, nil
	}
	return entry, err
}

func (s *FileStore) readEntry(ctx context.Context, path entryPath) (*model.DebugLogEntry, error) {
	meta, err := readMetadata(ctx, filepath.Join(path.dir, metadataFileName))
	if err != nil {
		return nil, fmt.Errorf("read debug log metadata: %w", err)
	}
	if meta.Version != formatVersion {
		return nil, fmt.Errorf("unsupported debug log format version: %d", meta.Version)
	}
	if meta.LogID != path.logID {
		return nil, fmt.Errorf("debug log id mismatch: directory=%d metadata=%d", path.logID, meta.LogID)
	}
	if meta.AuthTokenKey != path.tokenKey {
		return nil, fmt.Errorf("debug token key mismatch for log_id=%d", path.logID)
	}
	reqBody, err := readGzipFile(ctx, filepath.Join(path.dir, requestFileName))
	if err != nil {
		return nil, fmt.Errorf("read debug request body: %w", err)
	}
	respBody, err := readGzipFile(ctx, filepath.Join(path.dir, responseFileName))
	if err != nil {
		return nil, fmt.Errorf("read debug response body: %w", err)
	}
	return &model.DebugLogEntry{
		LogID:        meta.LogID,
		AuthTokenID:  meta.AuthTokenID,
		AuthTokenKey: meta.AuthTokenKey,
		CreatedAt:    meta.CreatedAt,
		ReqMethod:    meta.ReqMethod,
		ReqURL:       meta.ReqURL,
		ReqHeaders:   meta.ReqHeaders,
		ReqBody:      reqBody,
		RespStatus:   meta.RespStatus,
		RespHeaders:  meta.RespHeaders,
		RespBody:     respBody,
	}, nil
}

func readMetadata(ctx context.Context, path string) (metadata, error) {
	data, err := readGzipFile(ctx, path)
	if err != nil {
		return metadata{}, err
	}
	var meta metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return metadata{}, err
	}
	return meta, nil
}

func readGzipFile(ctx context.Context, path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()

	var out bytes.Buffer
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := zr.Read(buffer)
		if n > 0 {
			_, _ = out.Write(buffer[:n])
		}
		if errors.Is(readErr, io.EOF) {
			return out.Bytes(), nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}

func (s *FileStore) findEntry(logID int64) (*entryPath, error) {
	if cached, ok := s.index.Load(logID); ok {
		tokenKey := cached.(string)
		path := entryPath{logID: logID, tokenKey: tokenKey, dir: s.entryDir(tokenKey, logID)}
		if _, err := os.Stat(filepath.Join(path.dir, metadataFileName)); err == nil {
			return &path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		s.index.Delete(logID)
	}

	tokenDirs, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list debug log directory: %w", err)
	}
	logName := strconv.FormatInt(logID, 10)
	for _, tokenDir := range tokenDirs {
		if !tokenDir.IsDir() || isLegacyLogDir(tokenDir.Name()) {
			continue
		}
		path := entryPath{logID: logID, tokenKey: tokenDir.Name(), dir: filepath.Join(s.dir, tokenDir.Name(), logName)}
		if _, err := os.Stat(filepath.Join(path.dir, metadataFileName)); err == nil {
			s.index.Store(logID, tokenDir.Name())
			return &path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return nil, nil
}

// List returns published entries across all token directories in log ID order.
func (s *FileStore) List(ctx context.Context, afterLogID int64, minCreatedAt int64, limit int) ([]EntryInfo, error) {
	paths, err := s.listEntryPaths(ctx, afterLogID, false)
	if err != nil {
		return nil, err
	}
	entries := make([]EntryInfo, 0, minPositive(limit, len(paths)))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return entries, err
		}
		metaPath := filepath.Join(path.dir, metadataFileName)
		meta, err := readMetadata(ctx, metaPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return entries, fmt.Errorf("read debug log %d metadata: %w", path.logID, err)
		}
		if meta.CreatedAt < minCreatedAt {
			continue
		}
		info, err := os.Stat(metaPath)
		if err != nil {
			return entries, fmt.Errorf("stat debug log %d metadata: %w", path.logID, err)
		}
		s.index.Store(path.logID, path.tokenKey)
		entries = append(entries, EntryInfo{
			LogID: path.logID, AuthTokenID: meta.AuthTokenID, AuthTokenKey: path.tokenKey,
			CreatedAt: meta.CreatedAt, ModTime: info.ModTime(),
		})
		if limit > 0 && len(entries) >= limit {
			break
		}
	}
	return entries, nil
}

func (s *FileStore) listEntryPaths(ctx context.Context, afterLogID int64, includeLegacy bool) ([]entryPath, error) {
	rootEntries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list debug log directory: %w", err)
	}
	var paths []entryPath
	for _, rootEntry := range rootEntries {
		if err := ctx.Err(); err != nil {
			return paths, err
		}
		if !rootEntry.IsDir() {
			continue
		}
		if id, err := strconv.ParseInt(rootEntry.Name(), 10, 64); err == nil && id > afterLogID {
			if includeLegacy {
				paths = append(paths, entryPath{logID: id, dir: filepath.Join(s.dir, rootEntry.Name()), legacy: true})
			}
			continue
		}
		children, err := os.ReadDir(filepath.Join(s.dir, rootEntry.Name()))
		if err != nil {
			return paths, fmt.Errorf("list debug token directory %s: %w", rootEntry.Name(), err)
		}
		for _, child := range children {
			if !child.IsDir() {
				continue
			}
			id, err := strconv.ParseInt(child.Name(), 10, 64)
			if err != nil || id <= afterLogID {
				continue
			}
			paths = append(paths, entryPath{
				logID: id, tokenKey: rootEntry.Name(),
				dir: filepath.Join(s.dir, rootEntry.Name(), child.Name()),
			})
		}
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i].logID < paths[j].logID })
	return paths, nil
}

func minPositive(limit, fallback int) int {
	if limit > 0 && limit < fallback {
		return limit
	}
	return fallback
}

func isLegacyLogDir(name string) bool {
	id, err := strconv.ParseInt(name, 10, 64)
	return err == nil && id > 0
}

// Cleanup removes expired Log ID directories and always clears the obsolete
// root/<log-id> layout. A configured token ID/key is never removed.
func (s *FileStore) Cleanup(ctx context.Context, policy CleanupPolicy) (int, error) {
	if s == nil {
		return 0, errors.New("debug log file store is nil")
	}
	paths, err := s.listEntryPaths(ctx, 0, true)
	if err != nil {
		return 0, err
	}
	preserveKey := ""
	if strings.TrimSpace(policy.PreserveTokenKey) != "" {
		preserveKey = NormalizeTokenKey(policy.PreserveTokenKey, policy.PreserveAuthTokenID)
	}
	removed := 0
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if policy.MaxDelete > 0 && removed >= policy.MaxDelete {
			break
		}
		if path.legacy {
			if err := os.RemoveAll(path.dir); err != nil {
				return removed, fmt.Errorf("remove legacy debug log %d: %w", path.logID, err)
			}
			removed++
			continue
		}
		if preserveKey != "" && path.tokenKey == preserveKey {
			continue
		}
		meta, metaErr := readMetadata(ctx, filepath.Join(path.dir, metadataFileName))
		if metaErr == nil {
			if policy.PreserveAuthTokenID > 0 && meta.AuthTokenID == policy.PreserveAuthTokenID {
				continue
			}
			if meta.CreatedAt >= policy.Cutoff.Unix() {
				continue
			}
		} else {
			info, statErr := os.Stat(path.dir)
			if statErr != nil {
				if errors.Is(statErr, os.ErrNotExist) {
					continue
				}
				return removed, statErr
			}
			if !info.ModTime().Before(policy.Cutoff) {
				continue
			}
		}
		if err := os.RemoveAll(path.dir); err != nil {
			return removed, fmt.Errorf("remove debug log %d: %w", path.logID, err)
		}
		s.index.Delete(path.logID)
		removed++
	}
	remaining := 0
	if policy.MaxDelete > 0 {
		remaining = policy.MaxDelete - removed
		if remaining <= 0 {
			s.removeEmptyTokenDirs()
			return removed, nil
		}
	}
	staleRemoved, err := s.removeStaleTempDirs(ctx, policy.Cutoff, remaining)
	if err != nil {
		return removed, err
	}
	removed += staleRemoved
	s.removeEmptyTokenDirs()
	return removed, nil
}

func (s *FileStore) removeStaleTempDirs(ctx context.Context, cutoff time.Time, maxDelete int) (int, error) {
	staleBefore := time.Now().Add(-10 * time.Minute)
	if cutoff.Before(staleBefore) {
		staleBefore = cutoff
	}
	rootEntries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, rootEntry := range rootEntries {
		if !rootEntry.IsDir() || isLegacyLogDir(rootEntry.Name()) {
			continue
		}
		tokenDir := filepath.Join(s.dir, rootEntry.Name())
		children, err := os.ReadDir(tokenDir)
		if err != nil {
			return removed, err
		}
		for _, child := range children {
			if err := ctx.Err(); err != nil {
				return removed, err
			}
			if !child.IsDir() || !strings.HasPrefix(child.Name(), ".tmp-") {
				continue
			}
			info, err := child.Info()
			if err != nil {
				return removed, err
			}
			if !info.ModTime().Before(staleBefore) {
				continue
			}
			if err := os.RemoveAll(filepath.Join(tokenDir, child.Name())); err != nil {
				return removed, err
			}
			removed++
			if maxDelete > 0 && removed >= maxDelete {
				return removed, nil
			}
		}
	}
	return removed, nil
}

func (s *FileStore) removeEmptyTokenDirs() {
	rootEntries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, rootEntry := range rootEntries {
		if !rootEntry.IsDir() || isLegacyLogDir(rootEntry.Name()) {
			continue
		}
		path := filepath.Join(s.dir, rootEntry.Name())
		children, err := os.ReadDir(path)
		if err == nil && len(children) == 0 {
			_ = os.Remove(path)
		}
	}
}

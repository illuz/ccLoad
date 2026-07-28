package debuganalysis

import (
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
	"time"

	"ccLoad/internal/debuglog"
)

const AnalysisFileName = "analysis.json.gz"

var ErrDebugLogNotFound = errors.New("debug log not found")

type Runner struct {
	Store     *debuglog.FileStore
	OutputDir string
}

type BatchResult struct {
	Analyzed int
	Skipped  int
	MaxLogID int64
}

func (r *Runner) AnalyzeBatch(ctx context.Context, afterLogID, minCreatedAt int64, limit int, force bool) (BatchResult, error) {
	if err := r.validate(); err != nil {
		return BatchResult{}, err
	}
	entries, err := r.Store.List(ctx, afterLogID, minCreatedAt, limit)
	if err != nil {
		return BatchResult{}, err
	}
	result := BatchResult{MaxLogID: afterLogID}
	for _, info := range entries {
		if info.LogID > result.MaxLogID {
			result.MaxLogID = info.LogID
		}
		outputPath := OutputPath(r.OutputDir, info.AuthTokenKey, info.LogID)
		if !force {
			if outputInfo, statErr := os.Stat(outputPath); statErr == nil && !outputInfo.ModTime().Before(info.ModTime) {
				result.Skipped++
				continue
			} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				return result, fmt.Errorf("stat analysis output %d: %w", info.LogID, statErr)
			}
		}
		if err := r.analyzeID(ctx, info.LogID); err != nil {
			return result, err
		}
		result.Analyzed++
	}
	return result, nil
}

func (r *Runner) AnalyzeID(ctx context.Context, logID int64) error {
	if err := r.validate(); err != nil {
		return err
	}
	return r.analyzeID(ctx, logID)
}

func (r *Runner) analyzeID(ctx context.Context, logID int64) error {
	entry, err := r.Store.Get(ctx, logID)
	if err != nil {
		return fmt.Errorf("read debug log %d: %w", logID, err)
	}
	if entry == nil {
		return fmt.Errorf("%w: %d", ErrDebugLogNotFound, logID)
	}
	analysis := Analyze(entry, r.Store.Dir())
	path := OutputPath(r.OutputDir, entry.AuthTokenKey, logID)
	if err := writeJSONGzipAtomically(ctx, r.OutputDir, path, analysis); err != nil {
		return fmt.Errorf("write analysis %d: %w", logID, err)
	}
	return nil
}

func (r *Runner) validate() error {
	if r == nil || r.Store == nil {
		return errors.New("debug analysis runner has no input store")
	}
	if strings.TrimSpace(r.OutputDir) == "" {
		return errors.New("debug analysis output directory is empty")
	}
	return nil
}

// OutputPath returns <root>/<token-key>/<log-id>/analysis.json.gz.
func OutputPath(outputDir, tokenKey string, logID int64) string {
	tokenKey = debuglog.NormalizeTokenKey(tokenKey, 0)
	return filepath.Join(outputDir, tokenKey, strconv.FormatInt(logID, 10), AnalysisFileName)
}

func writeJSONGzipAtomically(ctx context.Context, outputDir, finalPath string, value any) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	entryDir := filepath.Dir(finalPath)
	if err := os.MkdirAll(entryDir, 0o700); err != nil {
		return err
	}
	for _, dir := range []string{outputDir, filepath.Dir(entryDir), entryDir} {
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(entryDir, ".analysis-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	zw, err := gzip.NewWriterLevel(tmp, gzip.BestSpeed)
	if err != nil {
		_ = tmp.Close()
		return err
	}
	encoder := json.NewEncoder(zw)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		if removeErr := os.Remove(finalPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return err
		}
		if retryErr := os.Rename(tmpPath, finalPath); retryErr != nil {
			return retryErr
		}
	}
	return nil
}

// FindOutputPath locates a globally unique Log ID across token directories.
func FindOutputPath(outputDir string, logID int64) (string, error) {
	rootEntries, err := os.ReadDir(outputDir)
	if errors.Is(err, os.ErrNotExist) {
		return "", os.ErrNotExist
	}
	if err != nil {
		return "", err
	}
	logName := strconv.FormatInt(logID, 10)
	for _, tokenDir := range rootEntries {
		if !tokenDir.IsDir() {
			continue
		}
		path := filepath.Join(outputDir, tokenDir.Name(), logName, AnalysisFileName)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", os.ErrNotExist
}

// ReadOutput expands one gzip JSON result.
func ReadOutput(ctx context.Context, path string) ([]byte, error) {
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

	reader := &contextReader{ctx: ctx, reader: zr}
	return io.ReadAll(reader)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func MaxOutputLogID(outputDir string) (int64, error) {
	rootEntries, err := os.ReadDir(outputDir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var maxID int64
	for _, tokenDir := range rootEntries {
		if !tokenDir.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(outputDir, tokenDir.Name()))
		if err != nil {
			return 0, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			id, err := strconv.ParseInt(entry.Name(), 10, 64)
			if err != nil || id <= maxID {
				continue
			}
			if _, err := os.Stat(filepath.Join(outputDir, tokenDir.Name(), entry.Name(), AnalysisFileName)); err == nil {
				maxID = id
			} else if !errors.Is(err, os.ErrNotExist) {
				return 0, err
			}
		}
	}
	return maxID, nil
}

func CleanupOutputs(ctx context.Context, outputDir, sourceDir string, cutoff time.Time, maxDelete int, sleep time.Duration) (int, error) {
	rootEntries, err := os.ReadDir(outputDir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	type candidate struct {
		dir      string
		tokenKey string
		logID    string
		modTime  time.Time
	}
	var candidates []candidate
	var legacyPaths []string
	for _, tokenDir := range rootEntries {
		if !tokenDir.IsDir() {
			if strings.HasSuffix(tokenDir.Name(), ".json") {
				legacyPaths = append(legacyPaths, filepath.Join(outputDir, tokenDir.Name()))
			}
			continue
		}
		tokenPath := filepath.Join(outputDir, tokenDir.Name())
		entries, err := os.ReadDir(tokenPath)
		if err != nil {
			return 0, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if _, err := strconv.ParseInt(entry.Name(), 10, 64); err != nil {
				continue
			}
			entryDir := filepath.Join(tokenPath, entry.Name())
			info, err := os.Stat(filepath.Join(entryDir, AnalysisFileName))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return 0, err
			}
			if info.ModTime().Before(cutoff) {
				candidates = append(candidates, candidate{
					dir: entryDir, tokenKey: tokenDir.Name(), logID: entry.Name(), modTime: info.ModTime(),
				})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].modTime.Before(candidates[j].modTime) })
	removed := 0
	remove := func(path string) error {
		if maxDelete > 0 && removed >= maxDelete {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		removed++
		if sleep > 0 {
			timer := time.NewTimer(sleep)
			select {
			case <-timer.C:
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return ctx.Err()
			}
		}
		return nil
	}
	for _, path := range legacyPaths {
		if maxDelete > 0 && removed >= maxDelete {
			break
		}
		if err := remove(path); err != nil {
			return removed, err
		}
	}
	for _, candidate := range candidates {
		if maxDelete > 0 && removed >= maxDelete {
			break
		}
		if sourceDir != "" {
			sourceEntryDir := filepath.Join(sourceDir, candidate.tokenKey, candidate.logID)
			if info, err := os.Stat(sourceEntryDir); err == nil && info.IsDir() {
				continue
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return removed, err
			}
		}
		if err := remove(candidate.dir); err != nil {
			return removed, err
		}
	}
	removeEmptyOutputDirs(outputDir)
	return removed, nil
}

func removeEmptyOutputDirs(outputDir string) {
	rootEntries, err := os.ReadDir(outputDir)
	if err != nil {
		return
	}
	for _, entry := range rootEntries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(outputDir, entry.Name())
		children, err := os.ReadDir(path)
		if err == nil && len(children) == 0 {
			_ = os.Remove(path)
		}
	}
}

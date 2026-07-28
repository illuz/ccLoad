package debuganalysis

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ccLoad/internal/debuglog"
	"ccLoad/internal/model"
)

func TestRunnerAnalyzeBatchWritesCompatibleJSON(t *testing.T) {
	t.Parallel()

	input := debuglog.NewFileStore(t.TempDir())
	output := t.TempDir()
	for _, id := range []int64{9, 2} {
		tokenKey := strings.Repeat(strconv.FormatInt(id, 10), 64)
		if err := input.Put(t.Context(), &model.DebugLogEntry{
			LogID: id, AuthTokenKey: tokenKey, CreatedAt: time.Now().Unix(),
			ReqBody:  []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
			RespBody: []byte(`{"choices":[{"message":{"role":"assistant","content":"world"}}]}`),
		}); err != nil {
			t.Fatalf("Put(%d): %v", id, err)
		}
	}

	runner := &Runner{Store: input, OutputDir: output}
	batch, err := runner.AnalyzeBatch(context.Background(), 0, 0, 10, false)
	if err != nil {
		t.Fatalf("AnalyzeBatch: %v", err)
	}
	if batch.Analyzed != 2 || batch.MaxLogID != 9 {
		t.Fatalf("batch=%+v", batch)
	}

	outputPath := OutputPath(output, strings.Repeat("2", 64), 2)
	compressed, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(compressed) < 2 || compressed[0] != 0x1f || compressed[1] != 0x8b {
		t.Fatalf("analysis output is not gzip: %x", compressed[:min(4, len(compressed))])
	}
	data, err := ReadOutput(t.Context(), outputPath)
	if err != nil {
		t.Fatalf("expand output: %v", err)
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result.LogID != 2 || result.FinalAIText != "world" || len(result.UserQuestions) != 1 {
		t.Fatalf("result=%+v", result)
	}

	batch, err = runner.AnalyzeBatch(context.Background(), 0, 0, 10, false)
	if err != nil {
		t.Fatalf("AnalyzeBatch second run: %v", err)
	}
	if batch.Analyzed != 0 || batch.Skipped != 2 {
		t.Fatalf("second batch=%+v", batch)
	}
	maxID, err := MaxOutputLogID(output)
	if err != nil || maxID != 9 {
		t.Fatalf("MaxOutputLogID=%d, err=%v", maxID, err)
	}
}

func TestRunnerAnalyzeIDNotFound(t *testing.T) {
	t.Parallel()

	runner := &Runner{Store: debuglog.NewFileStore(t.TempDir()), OutputDir: t.TempDir()}
	err := runner.AnalyzeID(t.Context(), 404)
	if !errors.Is(err, ErrDebugLogNotFound) {
		t.Fatalf("err=%v, want ErrDebugLogNotFound", err)
	}
}

func TestCleanupOutputsHonorsCutoffAndLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Now()
	for _, id := range []int64{1, 2} {
		path := OutputPath(dir, strings.Repeat("a", 64), id)
		if err := writeJSONGzipAtomically(t.Context(), dir, path, map[string]any{"log_id": id}); err != nil {
			t.Fatalf("write %d: %v", id, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-48 * time.Hour)
	for _, id := range []int64{1, 2} {
		path := OutputPath(dir, strings.Repeat("a", 64), id)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes %d: %v", id, err)
		}
	}
	removed, err := CleanupOutputs(t.Context(), dir, "", now.Add(-24*time.Hour), 1, 0)
	if err != nil {
		t.Fatalf("CleanupOutputs: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatalf("non-analysis file was removed: %v", err)
	}
}

func TestCleanupOutputsKeepsAnalysisWhileRawEntryExists(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	source := t.TempDir()
	tokenKey := strings.Repeat("b", 64)
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	for _, id := range []int64{1, 2} {
		path := OutputPath(output, tokenKey, id)
		if err := writeJSONGzipAtomically(t.Context(), output, path, map[string]any{"log_id": id}); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(source, tokenKey, "1"), 0o700); err != nil {
		t.Fatal(err)
	}
	removed, err := CleanupOutputs(t.Context(), output, source, now.Add(-24*time.Hour), 0, 0)
	if err != nil {
		t.Fatalf("CleanupOutputs: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d, want 1", removed)
	}
	if _, err := os.Stat(OutputPath(output, tokenKey, 1)); err != nil {
		t.Fatalf("analysis with raw source was removed: %v", err)
	}
	if _, err := os.Stat(OutputPath(output, tokenKey, 2)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphaned expired analysis was retained: %v", err)
	}
}

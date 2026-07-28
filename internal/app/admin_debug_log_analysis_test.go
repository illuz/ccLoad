package app

import (
	"net/http"
	"testing"
	"time"

	"ccLoad/internal/debuganalysis"
	"ccLoad/internal/debuglog"
	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

func TestHandleGetDebugLogAnalysisReadsTokenScopedGzip(t *testing.T) {
	input := debuglog.NewFileStore(t.TempDir())
	output := t.TempDir()
	entry := &model.DebugLogEntry{
		LogID: 91, AuthTokenKey: "token-key", CreatedAt: time.Now().Unix(),
		ReqBody:  []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
		RespBody: []byte(`{"choices":[{"message":{"role":"assistant","content":"world"}}]}`),
	}
	if err := input.Put(t.Context(), entry); err != nil {
		t.Fatalf("Put: %v", err)
	}
	runner := &debuganalysis.Runner{Store: input, OutputDir: output}
	if err := runner.AnalyzeID(t.Context(), entry.LogID); err != nil {
		t.Fatalf("AnalyzeID: %v", err)
	}
	t.Setenv("CCLOAD_DEBUG_ANALYSIS_DIR", output)

	srv := newInMemoryServer(t)
	c, w := newTestContext(t, newRequest(http.MethodGet, "/admin/debug-log-analysis/91", nil))
	c.Params = gin.Params{{Key: "log_id", Value: "91"}}
	srv.HandleGetDebugLogAnalysis(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := mustParseAPIResponse[map[string]any](t, w.Body.Bytes())
	if !resp.Success || resp.Data["final_ai_text"] != "world" {
		t.Fatalf("response=%+v", resp)
	}
}

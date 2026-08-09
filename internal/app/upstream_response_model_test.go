package app

import (
	"context"
	"strings"
	"testing"
)

func TestUpstreamResponseModelObserverTerminalWins(t *testing.T) {
	t.Parallel()

	observer := newUpstreamResponseModelObserver()
	observer.ObservePayload(map[string]any{
		"response": map[string]any{"model": "gpt-5.4"},
	}, "response.created")
	observer.ObservePayload(map[string]any{
		"type":     "response.completed",
		"response": map[string]any{"model": "gpt-5.4-final"},
	}, "response.completed")

	got, conflict := observer.Result()
	if got != "gpt-5.4-final" {
		t.Fatalf("response model=%q, want terminal model", got)
	}
	if !conflict {
		t.Fatal("expected conflicting upstream declarations to be recorded")
	}
}

func TestUpstreamResponseModelObserverExtractsProtocolShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{
			name:    "openai chat",
			payload: map[string]any{"model": "gpt-5.4"},
			want:    "gpt-5.4",
		},
		{
			name:    "anthropic message",
			payload: map[string]any{"message": map[string]any{"model": "claude-sonnet-4-5"}},
			want:    "claude-sonnet-4-5",
		},
		{
			name:    "gemini model version",
			payload: map[string]any{"modelVersion": "gemini-2.5-pro-001"},
			want:    "gemini-2.5-pro-001",
		},
		{
			name: "responses nested model has priority",
			payload: map[string]any{
				"model":    "outer-model",
				"response": map[string]any{"model": "response-model"},
			},
			want: "response-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			observer := newUpstreamResponseModelObserver()
			observer.ObservePayload(tt.payload, "")
			got, conflict := observer.Result()
			if got != tt.want {
				t.Fatalf("response model=%q, want %q", got, tt.want)
			}
			if conflict {
				t.Fatal("single declaration must not conflict")
			}
		})
	}
}

func TestRequestContextsKeepResponseModelAuditAttemptsIsolated(t *testing.T) {
	t.Parallel()

	server := &Server{}
	first := server.newRequestContextWithTimeouts(context.Background(), "/v1/chat/completions", nil, channelTypeTimeoutConfig{})
	second := server.newRequestContextWithTimeouts(context.Background(), "/v1/chat/completions", nil, channelTypeTimeoutConfig{})
	t.Cleanup(first.cleanup)
	t.Cleanup(second.cleanup)

	first.observeUpstreamResponseModelJSON([]byte(`{"model":"failed-attempt-model"}`), "")
	second.observeUpstreamResponseModelJSON([]byte(`{"model":"successful-attempt-model"}`), "")

	firstModel, _ := first.responseModelObserver.Result()
	secondModel, secondConflict := second.responseModelObserver.Result()
	if firstModel != "failed-attempt-model" {
		t.Fatalf("first attempt model=%q", firstModel)
	}
	if secondModel != "successful-attempt-model" || secondConflict {
		t.Fatalf("second attempt audit=(%q, %v)", secondModel, secondConflict)
	}
	if first.responseModelObserver == second.responseModelObserver {
		t.Fatal("forward attempts must not share a response model observer")
	}
}

func TestUsageParsersObserveUpstreamResponseModel(t *testing.T) {
	t.Parallel()

	sseObserver := newUpstreamResponseModelObserver()
	sseParser := newSSEUsageParser("openai", sseObserver)
	sse := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"model\":\"gpt-5.4\"}}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5.4\"}}\n\n"
	if err := sseParser.Feed([]byte(sse)); err != nil {
		t.Fatalf("feed SSE: %v", err)
	}
	if got, _ := sseObserver.Result(); got != "gpt-5.4" {
		t.Fatalf("SSE response model=%q, want gpt-5.4", got)
	}

	jsonObserver := newUpstreamResponseModelObserver()
	jsonParser := newJSONUsageParser("anthropic", jsonObserver)
	if err := jsonParser.Feed([]byte(`{"message":{"model":"claude-sonnet-4-5"}}`)); err != nil {
		t.Fatalf("feed JSON: %v", err)
	}
	jsonParser.GetUsage()
	if got, _ := jsonObserver.Result(); got != "claude-sonnet-4-5" {
		t.Fatalf("JSON response model=%q, want claude-sonnet-4-5", got)
	}
}

func TestUpstreamResponseModelMismatch(t *testing.T) {
	t.Parallel()

	if got := upstreamResponseModelMismatch("gpt-5.4", ""); got != nil {
		t.Fatalf("missing response model mismatch=%v, want nil", *got)
	}
	if got := upstreamResponseModelMismatch("gpt-5.4", "GPT-5.4"); got == nil || *got {
		t.Fatalf("case-insensitive match mismatch=%v, want false", got)
	}
	if got := upstreamResponseModelMismatch("gpt-5.4", "gpt-5.4-mini"); got == nil || !*got {
		t.Fatalf("different model mismatch=%v, want true", got)
	}
	if got := upstreamSentModel("gpt-5.4", "gpt-5.4-upstream"); got != "gpt-5.4-upstream" {
		t.Fatalf("sent model=%q, want actual upstream model", got)
	}
}

func TestUpstreamResponseModelTruncatesToStorageLimit(t *testing.T) {
	t.Parallel()

	modelName := strings.Repeat("m", maxUpstreamResponseModelLength+1)
	observer := newUpstreamResponseModelObserver()
	observer.ObservePayload(map[string]any{"model": modelName}, "")
	got, _ := observer.Result()
	if len([]rune(got)) != maxUpstreamResponseModelLength {
		t.Fatalf("model length=%d, want %d", len([]rune(got)), maxUpstreamResponseModelLength)
	}
}

func TestBuildLogEntryRecordsUpstreamResponseModelAudit(t *testing.T) {
	t.Parallel()

	entry := buildLogEntry(logEntryParams{
		RequestModel: "gpt-5.4",
		ActualModel:  "gpt-5.4-upstream",
		StatusCode:   200,
		Result: &fwResult{
			UpstreamResponseModel: "gpt-5.4-fallback",
		},
	})
	if entry.UpstreamResponseModel != "gpt-5.4-fallback" {
		t.Fatalf("upstream response model=%q", entry.UpstreamResponseModel)
	}
	if entry.UpstreamModelMismatch == nil || !*entry.UpstreamModelMismatch {
		t.Fatalf("mismatch=%v, want true", entry.UpstreamModelMismatch)
	}

	matched := false
	matchedEntry := buildLogEntry(logEntryParams{
		RequestModel: "gpt-5.4",
		StatusCode:   200,
		Result:       &fwResult{UpstreamResponseModel: "gpt-5.4"},
	})
	matched = matchedEntry.UpstreamModelMismatch != nil && !*matchedEntry.UpstreamModelMismatch
	if !matched {
		t.Fatalf("matched response audit=%v, want false", matchedEntry.UpstreamModelMismatch)
	}

}

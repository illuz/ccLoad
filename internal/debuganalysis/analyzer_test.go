package debuganalysis

import (
	"encoding/base64"
	"testing"

	"ccLoad/internal/model"
)

func TestExtractBase64ImagesSupportedFormatsAndDeduplication(t *testing.T) {
	t.Parallel()

	pngData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	jpegData := base64.StdEncoding.EncodeToString([]byte("jpeg-bytes"))
	payload := map[string]any{
		"messages": []any{map[string]any{
			"content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + pngData}},
				map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/jpeg", "data": jpegData}},
				map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": pngData}},
			},
		}},
	}

	images := ExtractBase64Images(payload, "input", MaxAnalysisImages)
	if len(images) != 2 {
		t.Fatalf("len(images)=%d, want 2: %+v", len(images), images)
	}
	if images[0].MIMEType != "image/png" || images[1].MIMEType != "image/jpeg" {
		t.Fatalf("MIME types=%q,%q", images[0].MIMEType, images[1].MIMEType)
	}
	if images[0].Data != pngData || images[1].Data != jpegData {
		t.Fatal("image payload was not preserved canonically")
	}
}

func TestExtractBase64ImagesRejectsInvalidData(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"svg":        "data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=",
		"invalid":    "data:image/png;base64,not-valid!",
		"wrong_mime": map[string]any{"media_type": "text/plain", "data": base64.StdEncoding.EncodeToString([]byte("png"))},
		"type":       map[string]any{"unexpected": "object"},
	}
	if images := ExtractBase64Images(payload, "output", MaxAnalysisImages); len(images) != 0 {
		t.Fatalf("images=%+v, want none", images)
	}
}

func TestExtractBase64ImagesGenerationAndMIMEInference(t *testing.T) {
	t.Parallel()

	generated := base64.StdEncoding.EncodeToString([]byte("generated"))
	images := ExtractBase64Images(map[string]any{"type": "image_generation_call", "result": generated}, "output", MaxAnalysisImages)
	if len(images) != 1 || images[0].MIMEType != "image/png" {
		t.Fatalf("generated images=%+v", images)
	}

	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nimage-data"))
	images = ExtractBase64Images(map[string]any{"data": []any{map[string]any{"b64_json": png}}}, "output", MaxAnalysisImages)
	if len(images) != 1 || images[0].MIMEType != "image/png" {
		t.Fatalf("inferred images=%+v", images)
	}
}

func TestAnalyzeExtractsQuestionsStreamingTextToolsAndPaths(t *testing.T) {
	t.Parallel()

	entry := &model.DebugLogEntry{
		LogID:     99,
		CreatedAt: 1234,
		ReqBody: []byte(`{
  "messages":[{"role":"user","content":"Please edit internal/app/server.go"}],
  "tools":[],
  "tool_calls":[{"function":{"name":"apply_patch","arguments":"{\"file_path\":\"internal/app/server.go\"}"}}]
}`),
		RespBody: []byte("data: {\"type\":\"response.output_text.delta\",\"item_id\":\"item-1\",\"delta\":\"done \"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"item-1\",\"delta\":\"now\"}\n\n" +
			"data: [DONE]\n\n"),
	}

	result := Analyze(entry, "/tmp/debug-logs")
	if len(result.UserQuestions) != 1 || result.UserQuestions[0].Content != "Please edit internal/app/server.go" {
		t.Fatalf("questions=%+v", result.UserQuestions)
	}
	if result.FinalAIText != "done now" {
		t.Fatalf("final_ai_text=%q", result.FinalAIText)
	}
	if len(result.ToolCalls) == 0 || result.ToolCalls[0].Name != "apply_patch" {
		t.Fatalf("tool_calls=%+v", result.ToolCalls)
	}
	if len(result.ToolFileTree.Paths) != 1 || result.ToolFileTree.Paths[0].Path != "internal/app/server.go" {
		t.Fatalf("paths=%+v", result.ToolFileTree.Paths)
	}
	if result.ToolFileTree.TreeText != "internal\n  app\n    server.go" {
		t.Fatalf("tree=%q", result.ToolFileTree.TreeText)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors=%v", result.Errors)
	}
}

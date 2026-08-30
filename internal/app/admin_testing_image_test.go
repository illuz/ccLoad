package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

func TestImageGenerationTestRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request imageGenerationTestRequest
		wantErr bool
	}{
		{
			name: "images options",
			request: imageGenerationTestRequest{
				GenerationAPI: " IMAGES ",
				Model:         " gpt-image-1 ",
				Prompt:        " draw a lighthouse ",
				Size:          "1024x1536",
				Quality:       "HIGH",
				Background:    "Transparent",
				OutputFormat:  "WEBP",
			},
		},
		{
			name: "chat completions options",
			request: imageGenerationTestRequest{
				GenerationAPI: imageGenerationAPIChatCompletions,
				Model:         "gemini-image",
				Prompt:        "draw a lighthouse",
				Size:          "3:2@2K",
			},
		},
		{
			name: "missing model",
			request: imageGenerationTestRequest{
				GenerationAPI: imageGenerationAPIImages,
				Prompt:        "draw a lighthouse",
			},
			wantErr: true,
		},
		{
			name: "missing prompt",
			request: imageGenerationTestRequest{
				GenerationAPI: imageGenerationAPIImages,
				Model:         "gpt-image-1",
			},
			wantErr: true,
		},
		{
			name: "unknown api",
			request: imageGenerationTestRequest{
				GenerationAPI: "responses",
				Model:         "gpt-image-1",
				Prompt:        "draw a lighthouse",
			},
			wantErr: true,
		},
		{
			name: "negative key index",
			request: imageGenerationTestRequest{
				GenerationAPI: imageGenerationAPIImages,
				Model:         "gpt-image-1",
				Prompt:        "draw a lighthouse",
				KeyIndex:      -1,
			},
			wantErr: true,
		},
		{
			name: "images rejects aspect ratio size",
			request: imageGenerationTestRequest{
				GenerationAPI: imageGenerationAPIImages,
				Model:         "gpt-image-1",
				Prompt:        "draw a lighthouse",
				Size:          "3:2@2k",
			},
			wantErr: true,
		},
		{
			name: "chat rejects pixel size",
			request: imageGenerationTestRequest{
				GenerationAPI: imageGenerationAPIChatCompletions,
				Model:         "gemini-image",
				Prompt:        "draw a lighthouse",
				Size:          "1024x1024",
			},
			wantErr: true,
		},
		{
			name: "chat rejects images-only options",
			request: imageGenerationTestRequest{
				GenerationAPI: imageGenerationAPIChatCompletions,
				Model:         "gemini-image",
				Prompt:        "draw a lighthouse",
				Quality:       "high",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.request.GenerationAPI == imageGenerationAPIImages {
				if tt.request.Model != "gpt-image-1" || tt.request.Quality != "high" {
					t.Fatalf("Validate() did not normalize request: %+v", tt.request)
				}
			}
		})
	}
}

func TestImageGenerationRequestBody(t *testing.T) {
	body, err := imageGenerationRequestBody("gpt-image-2", &imageGenerationTestRequest{
		Prompt:       "draw a lighthouse",
		Size:         "auto",
		Quality:      "high",
		Background:   "transparent",
		OutputFormat: "webp",
	})
	if err != nil {
		t.Fatalf("imageGenerationRequestBody() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if payload["model"] != "gpt-image-2" || payload["prompt"] != "draw a lighthouse" {
		t.Fatalf("routing fields = %#v", payload)
	}
	if _, exists := payload["size"]; exists {
		t.Fatalf("automatic size should be omitted: %#v", payload)
	}
	if payload["quality"] != "high" || payload["background"] != "transparent" || payload["output_format"] != "webp" {
		t.Fatalf("image options = %#v", payload)
	}
}

func TestChatCompletionsImageOptions(t *testing.T) {
	options := chatCompletionsImageOptions("3:2@2k")
	if options.AspectRatio != "3:2" || options.ImageSize != "2K" {
		t.Fatalf("chatCompletionsImageOptions() = %+v", options)
	}

	testRequest := imageGenerationChannelTestRequest(&imageGenerationTestRequest{
		GenerationAPI: imageGenerationAPIChatCompletions,
		Model:         "gemini-image",
		Prompt:        "draw a lighthouse",
		Size:          "3:2@2k",
	})
	if testRequest.Stream || testRequest.ImageGeneration == nil || testRequest.ProtocolTransform != "openai" {
		t.Fatalf("imageGenerationChannelTestRequest() = %+v", testRequest)
	}
}

func TestImageGenerationResponseParsing(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusOK, Status: "200 OK"}
	result := map[string]any{"success": false}
	parseImageGenerationResponse(result, response, []byte(`{
		"output_format":"webp",
		"data":[
			{"url":"https://example.com/image.webp","revised_prompt":"a lighthouse at dusk"},
			{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB"}
		]
	}`), "auto")

	if success, _ := result["success"].(bool); !success {
		t.Fatalf("parseImageGenerationResponse() failed: %#v", result)
	}
	images, ok := result["images"].([]map[string]any)
	if !ok || len(images) != 2 {
		t.Fatalf("images = %#v", result["images"])
	}
	if images[0]["url"] != "https://example.com/image.webp" || images[1]["mime_type"] != "image/png" {
		t.Fatalf("normalized images = %#v", images)
	}
}

func TestNormalizeChatCompletionsImageResult(t *testing.T) {
	result := normalizeChatCompletionsImageResult(map[string]any{
		"success": true,
		"api_response": map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{
						"images": []any{
							map[string]any{
								"image_url": map[string]any{
									"url": "data:image/webp;base64,aW1hZ2U=",
								},
							},
						},
					},
				},
			},
		},
		"upstream_response_body": "large duplicate",
		"cost_usd":               1.25,
	})

	if success, _ := result["success"].(bool); !success {
		t.Fatalf("normalizeChatCompletionsImageResult() failed: %#v", result)
	}
	images, ok := result["images"].([]map[string]any)
	if !ok || len(images) != 1 || images[0]["b64_json"] != "aW1hZ2U=" || images[0]["mime_type"] != "image/webp" {
		t.Fatalf("normalized chat images = %#v", result["images"])
	}
	if result["output_format"] != "webp" {
		t.Fatalf("output_format = %#v", result["output_format"])
	}
	for _, key := range []string{"api_response", "upstream_response_body", "cost_usd"} {
		if _, exists := result[key]; exists {
			t.Fatalf("successful response retained %q: %#v", key, result)
		}
	}
}

func TestReadLimitedImageGenerationResponse(t *testing.T) {
	body, err := readLimitedImageGenerationResponse(strings.NewReader("12345"), 4)
	if err == nil || string(body) != "1234" {
		t.Fatalf("readLimitedImageGenerationResponse() body=%q err=%v", body, err)
	}
	if !strings.Contains(err.Error(), "4") {
		t.Fatalf("limit error = %q", err)
	}
}

func TestHandleChannelImageGenerationForwardsImagesRequest(t *testing.T) {
	var gotPath, gotAuthorization, gotCustomHeader string
	var gotBody map[string]any
	upstream := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		gotCustomHeader = r.Header.Get("X-Image-Test")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"url":"https://example.com/generated.webp"}],"output_format":"webp"}`)
	}))
	defer upstream.Close()

	srv := newInMemoryServer(t)
	srv.client = upstream.Client()
	created, err := srv.store.CreateConfig(context.Background(), &model.Config{
		Name:        "images-admin-test",
		ChannelType: "openai",
		URL:         upstream.URL,
		Priority:    1,
		Enabled:     true,
		ModelEntries: []model.ModelEntry{{
			Model:         "image-alias",
			RedirectModel: "gpt-image-2",
		}},
		CustomRequestRules: &model.CustomRequestRules{
			Headers: []model.CustomHeaderRule{{
				Action: model.RuleActionOverride,
				Name:   "X-Image-Test",
				Value:  "enabled",
			}},
			Body: []model.CustomBodyRule{{
				Action: model.RuleActionOverride,
				Path:   "quality",
				Value:  json.RawMessage(`"low"`),
			}},
		},
	})
	if err != nil {
		t.Fatalf("CreateConfig() error = %v", err)
	}
	if err := srv.store.CreateAPIKeysBatch(context.Background(), []*model.APIKey{
		{ChannelID: created.ID, KeyIndex: 0, APIKey: "sk-unused"},
		{ChannelID: created.ID, KeyIndex: 3, APIKey: "sk-image-test"},
	}); err != nil {
		t.Fatalf("CreateAPIKeysBatch() error = %v", err)
	}

	request := newJSONRequest(t, http.MethodPost, fmt.Sprintf("/admin/channels/%d/images/generations", created.ID), map[string]any{
		"generation_api": imageGenerationAPIImages,
		"model":          "image-alias",
		"prompt":         "draw a lighthouse",
		"quality":        "high",
		"key_index":      3,
	})
	c, recorder := newTestContext(t, request)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", created.ID)}}
	srv.HandleChannelImageGeneration(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotPath != imageGenerationPath || gotAuthorization != "Bearer sk-image-test" || gotCustomHeader != "enabled" {
		t.Fatalf("upstream request path=%q auth=%q custom=%q", gotPath, gotAuthorization, gotCustomHeader)
	}
	if gotBody["model"] != "gpt-image-2" || gotBody["quality"] != "low" {
		t.Fatalf("upstream body = %#v", gotBody)
	}
	response := mustParseAPIResponse[map[string]any](t, recorder.Body.Bytes())
	if success, _ := response.Data["success"].(bool); !success {
		t.Fatalf("image generation failed: %#v", response.Data)
	}
	if response.Data["actual_model"] != "gpt-image-2" || response.Data["tested_key_index"] != float64(3) {
		t.Fatalf("routing metadata = %#v", response.Data)
	}
}

func TestHandleChannelImageGenerationUsesChatCompletions(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	upstream := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{"message":{"role":"assistant","content":null,"images":[{"image_url":{"url":"data:image/png;base64,aW1hZ2U="}}]}}]
		}`)
	}))
	defer upstream.Close()

	srv := newInMemoryServer(t)
	srv.client = upstream.Client()
	created, err := srv.store.CreateConfig(context.Background(), &model.Config{
		Name:         "chat-images-admin-test",
		ChannelType:  "openai",
		URL:          upstream.URL,
		Priority:     1,
		Enabled:      true,
		ModelEntries: []model.ModelEntry{{Model: "gemini-image", RedirectModel: "gemini-image-upstream"}},
	})
	if err != nil {
		t.Fatalf("CreateConfig() error = %v", err)
	}
	if err := srv.store.CreateAPIKeysBatch(context.Background(), []*model.APIKey{{
		ChannelID: created.ID,
		KeyIndex:  0,
		APIKey:    "sk-image-test",
	}}); err != nil {
		t.Fatalf("CreateAPIKeysBatch() error = %v", err)
	}

	request := newJSONRequest(t, http.MethodPost, fmt.Sprintf("/admin/channels/%d/images/generations", created.ID), map[string]any{
		"generation_api": imageGenerationAPIChatCompletions,
		"model":          "gemini-image",
		"prompt":         "draw a lighthouse",
		"size":           "3:2@2k",
	})
	c, recorder := newTestContext(t, request)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", created.ID)}}
	srv.HandleChannelImageGeneration(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("upstream path = %q", gotPath)
	}
	if gotBody["model"] != "gemini-image-upstream" {
		t.Fatalf("upstream model = %#v", gotBody["model"])
	}
	modalities, _ := gotBody["modalities"].([]any)
	imageConfig, _ := gotBody["image_config"].(map[string]any)
	if len(modalities) != 1 || modalities[0] != "image" || imageConfig["aspect_ratio"] != "3:2" || imageConfig["image_size"] != "2K" {
		t.Fatalf("chat image options = %#v", gotBody)
	}
	response := mustParseAPIResponse[map[string]any](t, recorder.Body.Bytes())
	if success, _ := response.Data["success"].(bool); !success {
		t.Fatalf("chat image generation failed: %#v", response.Data)
	}
	if response.Data["actual_model"] != "gemini-image-upstream" || response.Data["output_format"] != "png" {
		t.Fatalf("response metadata = %#v", response.Data)
	}
}

func TestChannelImageGenerationFallsBackAcrossURLs(t *testing.T) {
	var firstCalls, secondCalls atomic.Int32
	first := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"error":{"type":"api_error","message":"upstream overloaded"}}`)
	}))
	defer first.Close()
	second := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"aW1hZ2U="}]}`)
	}))
	defer second.Close()

	srv := newInMemoryServer(t)
	srv.client = first.Client()
	srv.urlSelector = nil
	result := srv.testChannelImageGeneration(context.Background(), &model.Config{
		ID:  1,
		URL: first.URL + "\n" + second.URL,
	}, "sk-test", &imageGenerationTestRequest{
		GenerationAPI: imageGenerationAPIImages,
		Model:         "gpt-image-1",
		Prompt:        "draw a lighthouse",
	})

	if success, _ := result["success"].(bool); !success {
		t.Fatalf("fallback result = %#v", result)
	}
	if firstCalls.Load() != 1 || secondCalls.Load() != 1 {
		t.Fatalf("fallback calls first=%d second=%d", firstCalls.Load(), secondCalls.Load())
	}
}

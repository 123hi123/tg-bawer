package grok

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("test-key", "", "", "", "")
	if c.baseURL != DefaultBaseURL {
		t.Fatalf("expected %s, got %s", DefaultBaseURL, c.baseURL)
	}
	if c.imgModel != DefaultImgModel {
		t.Fatalf("expected %s, got %s", DefaultImgModel, c.imgModel)
	}
	if c.editModel != DefaultEditModel {
		t.Fatalf("expected %s, got %s", DefaultEditModel, c.editModel)
	}
	if c.videoModel != DefaultVideoModel {
		t.Fatalf("expected %s, got %s", DefaultVideoModel, c.videoModel)
	}
}

func TestNewClient_Custom(t *testing.T) {
	c := NewClient("key", "http://custom:9000/", "img-model", "edit-model", "video-model")
	if c.baseURL != "http://custom:9000" {
		t.Fatalf("expected trailing slash removed, got %s", c.baseURL)
	}
	if c.imgModel != "img-model" {
		t.Fatalf("expected img-model, got %s", c.imgModel)
	}
}

func TestAvailable(t *testing.T) {
	c := NewClient("", "", "", "", "")
	if c.Available() {
		t.Fatal("expected not available with empty key")
	}
	c2 := NewClient("key", "", "", "", "")
	if !c2.Available() {
		t.Fatal("expected available with key")
	}
}

func TestModelGetters_Defaults(t *testing.T) {
	c := NewClient("key", "", "", "", "")
	if c.ImageModel() != DefaultImgModel {
		t.Fatalf("expected %s, got %s", DefaultImgModel, c.ImageModel())
	}
	if c.EditModel() != DefaultEditModel {
		t.Fatalf("expected %s, got %s", DefaultEditModel, c.EditModel())
	}
	if c.VideoModel() != DefaultVideoModel {
		t.Fatalf("expected %s, got %s", DefaultVideoModel, c.VideoModel())
	}
}

func TestModelGetters_Custom(t *testing.T) {
	c := NewClient("key", "", "my-img", "my-edit", "my-video")
	if c.ImageModel() != "my-img" {
		t.Fatalf("expected my-img, got %s", c.ImageModel())
	}
	if c.EditModel() != "my-edit" {
		t.Fatalf("expected my-edit, got %s", c.EditModel())
	}
	if c.VideoModel() != "my-video" {
		t.Fatalf("expected my-video, got %s", c.VideoModel())
	}
}

func TestExtractImageURL_ChatCompletions(t *testing.T) {
	resp := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"content": "https://example.com/image.png",
				},
			},
		},
	}
	body, _ := json.Marshal(resp)
	u, err := extractImageURL(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "https://example.com/image.png" {
		t.Fatalf("unexpected URL: %s", u)
	}
}

func TestExtractImageURL_DataArray(t *testing.T) {
	resp := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{
				"url": "https://example.com/edited.png",
			},
		},
	}
	body, _ := json.Marshal(resp)
	u, err := extractImageURL(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "https://example.com/edited.png" {
		t.Fatalf("unexpected URL: %s", u)
	}
}

func TestExtractEditImageURL(t *testing.T) {
	resp := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{
				"url": "https://example.com/result.png",
			},
		},
	}
	body, _ := json.Marshal(resp)
	u, err := extractEditImageURL(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "https://example.com/result.png" {
		t.Fatalf("unexpected URL: %s", u)
	}
}

func TestExtractVideoURL(t *testing.T) {
	resp := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"content": "https://example.com/video.mp4",
				},
			},
		},
	}
	body, _ := json.Marshal(resp)
	u, err := extractVideoURL(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "https://example.com/video.mp4" {
		t.Fatalf("unexpected URL: %s", u)
	}
}

func TestExtractImageURL_NoData(t *testing.T) {
	body := []byte(`{}`)
	_, err := extractImageURL(body)
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestExtractVideoURL_ContentArray(t *testing.T) {
	resp := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": "https://example.com/video2.mp4",
							},
						},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(resp)
	u, err := extractVideoURL(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "https://example.com/video2.mp4" {
		t.Fatalf("unexpected URL: %s", u)
	}
}

func TestExtractVideoURL_DeltaHTMLChunk(t *testing.T) {
	resp := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"delta": map[string]interface{}{
					"content": `<video><source src="https://example.com/video3.mp4" type="video/mp4"></video>`,
				},
			},
		},
	}
	body, _ := json.Marshal(resp)
	u, err := extractVideoURL(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "https://example.com/video3.mp4" {
		t.Fatalf("unexpected URL: %s", u)
	}
}

func TestGenerateVideo_RequestBodyAndDownload(t *testing.T) {
	videoData := []byte("fake-mp4-data")

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Fatalf("unexpected authorization header: %s", got)
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}

			var req map[string]interface{}
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("unmarshal request: %v", err)
			}

			if req["model"] != DefaultVideoModel {
				t.Fatalf("unexpected model: %#v", req["model"])
			}

			messages := req["messages"].([]interface{})
			message := messages[0].(map[string]interface{})
			content := message["content"].([]interface{})
			if len(content) != 2 {
				t.Fatalf("expected 2 content items, got %d", len(content))
			}

			textItem := content[0].(map[string]interface{})
			if textItem["type"] != "text" || textItem["text"] != "turn around slowly" {
				t.Fatalf("unexpected text item: %#v", textItem)
			}

			imageItem := content[1].(map[string]interface{})
			if imageItem["type"] != "image_url" {
				t.Fatalf("unexpected image item type: %#v", imageItem)
			}
			imageURL := imageItem["image_url"].(map[string]interface{})["url"]
			if imageURL != "https://example.com/source.png" {
				t.Fatalf("unexpected image url: %#v", imageURL)
			}

			videoConfig := req["video_config"].(map[string]interface{})
			if videoConfig["aspect_ratio"] != DefaultVideoRatio {
				t.Fatalf("unexpected aspect ratio: %#v", videoConfig["aspect_ratio"])
			}
			if got := int(videoConfig["video_length"].(float64)); got != DefaultVideoLen {
				t.Fatalf("unexpected video length: %d", got)
			}
			if videoConfig["resolution_name"] != DefaultVideoRes {
				t.Fatalf("unexpected resolution: %#v", videoConfig["resolution_name"])
			}
			if videoConfig["preset"] != DefaultVideoPreset {
				t.Fatalf("unexpected preset: %#v", videoConfig["preset"])
			}

			resp := map[string]interface{}{
				"choices": []interface{}{
					map[string]interface{}{
						"message": map[string]interface{}{
							"content": []interface{}{
								map[string]interface{}{
									"type": "video_url",
									"video_url": map[string]interface{}{
										"url": server.URL + "/video.mp4",
									},
								},
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("encode response: %v", err)
			}

		case "/video.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			if _, err := w.Write(videoData); err != nil {
				t.Fatalf("write video: %v", err)
			}

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, "", "", "")
	result, err := client.GenerateVideo(context.Background(), "turn around slowly", "https://example.com/source.png")
	if err != nil {
		t.Fatalf("GenerateVideo returned error: %v", err)
	}
	if string(result.VideoData) != string(videoData) {
		t.Fatalf("unexpected video data: %q", string(result.VideoData))
	}
}

func TestGenerateVideo_UsesContentArrayWithoutImageAndParsesSSE(t *testing.T) {
	videoData := []byte("sse-video-data")

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}

			var req map[string]interface{}
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("unmarshal request: %v", err)
			}

			messages := req["messages"].([]interface{})
			message := messages[0].(map[string]interface{})
			content := message["content"].([]interface{})
			if len(content) != 1 {
				t.Fatalf("expected 1 content item without image, got %d", len(content))
			}
			textItem := content[0].(map[string]interface{})
			if textItem["type"] != "text" || textItem["text"] != "wave at camera" {
				t.Fatalf("unexpected text item: %#v", textItem)
			}

			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: progress\n")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"message\":{\"content\":\"still rendering\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"<video><source src=\\\""+server.URL+"/video.mp4\\\" type=\\\"video/mp4\\\"></video>\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n")

		case "/video.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			if _, err := w.Write(videoData); err != nil {
				t.Fatalf("write video: %v", err)
			}

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, "", "", "")
	result, err := client.GenerateVideo(context.Background(), "wave at camera", "")
	if err != nil {
		t.Fatalf("GenerateVideo returned error: %v", err)
	}
	if string(result.VideoData) != string(videoData) {
		t.Fatalf("unexpected video data: %q", string(result.VideoData))
	}
}

func TestGenerateVideo_SSEError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: error\n")
		_, _ = io.WriteString(w, "data: {\"error\":{\"message\":\"Video round 1/5 missing post_id\"}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n")
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, "", "", "")
	_, err := client.GenerateVideo(context.Background(), "broken", "")
	if err == nil {
		t.Fatal("expected GenerateVideo to return error")
	}
	if !strings.Contains(err.Error(), "missing post_id") {
		t.Fatalf("expected upstream error message, got: %v", err)
	}
}

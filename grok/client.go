package grok

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Client is the Grok API client for image generation, image editing, and video generation.
type Client struct {
	apiKey     string
	baseURL    string
	imgModel   string
	editModel  string
	videoModel string
	httpClient *http.Client
}

// ImageResult holds the generated image data.
type ImageResult struct {
	ImageData []byte
}

// VideoResult holds the generated video data.
type VideoResult struct {
	VideoData []byte
}

const (
	ServiceTypeGrok = "grok"

	DefaultBaseURL     = "http://127.0.0.1:8000"
	DefaultImgModel    = "grok-imagine-1.0"
	DefaultEditModel   = "grok-imagine-1.0-edit"
	DefaultVideoModel  = "grok-imagine-1.0-video"
	DefaultVideoRatio  = "9:16"
	DefaultVideoLen    = 30
	DefaultVideoRes    = "720p"
	DefaultVideoPreset = "custom"
)

var (
	sourceAttrPattern = regexp.MustCompile(`src=['"]([^'"]+)['"]`)
	httpURLPattern    = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// NewClient creates a new Grok client.
func NewClient(apiKey, baseURL, imgModel, editModel, videoModel string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if imgModel == "" {
		imgModel = DefaultImgModel
	}
	if editModel == "" {
		editModel = DefaultEditModel
	}
	if videoModel == "" {
		videoModel = DefaultVideoModel
	}
	return &Client{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		imgModel:   imgModel,
		editModel:  editModel,
		videoModel: videoModel,
		httpClient: &http.Client{
			Timeout: 6 * time.Minute,
		},
	}
}

// Available returns true if the Grok client has been configured with an API key.
func (c *Client) Available() bool {
	return strings.TrimSpace(c.apiKey) != ""
}

// ImageModel returns the image generation model name.
func (c *Client) ImageModel() string {
	return c.imgModel
}

// EditModel returns the image editing model name.
func (c *Client) EditModel() string {
	return c.editModel
}

// VideoModel returns the video generation model name.
func (c *Client) VideoModel() string {
	return c.videoModel
}

// GenerateImage generates an image from a text prompt.
func (c *Client) GenerateImage(ctx context.Context, prompt, size string) (*ImageResult, error) {
	if size == "" {
		size = "1024x1024"
	}

	requestBody := map[string]interface{}{
		"model": c.imgModel,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"image_config": map[string]interface{}{
			"n":               1,
			"size":            size,
			"response_format": "url",
		},
		"stream": false,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("grok: marshal request: %w", err)
	}

	endpoint := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("grok: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok: send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("grok: read response: %w", err)
	}

	if resp.StatusCode != 200 {
		payloads, _ := extractJSONPayloads(body)
		return nil, buildAPIError(body, payloads, resp.StatusCode)
	}

	payloads, err := extractJSONPayloads(body)
	if err != nil {
		return nil, err
	}
	if err := detectAPIError(payloads); err != nil {
		return nil, err
	}

	imageURL, err := extractImageURLFromPayloads(payloads)
	if err != nil {
		return nil, err
	}

	imageData, err := downloadURL(ctx, imageURL)
	if err != nil {
		return nil, fmt.Errorf("grok: download image: %w", err)
	}

	if len(imageData) == 0 {
		return nil, fmt.Errorf("grok: downloaded image is empty")
	}

	return &ImageResult{ImageData: imageData}, nil
}

// EditImage edits an image with a text prompt.
func (c *Client) EditImage(ctx context.Context, imageData []byte, prompt, size string) (*ImageResult, error) {
	if size == "" {
		size = "1024x1024"
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("model", c.editModel); err != nil {
		return nil, fmt.Errorf("grok: write model field: %w", err)
	}
	if err := writer.WriteField("prompt", prompt); err != nil {
		return nil, fmt.Errorf("grok: write prompt field: %w", err)
	}
	if err := writer.WriteField("n", "1"); err != nil {
		return nil, fmt.Errorf("grok: write n field: %w", err)
	}
	if err := writer.WriteField("size", size); err != nil {
		return nil, fmt.Errorf("grok: write size field: %w", err)
	}
	if err := writer.WriteField("response_format", "url"); err != nil {
		return nil, fmt.Errorf("grok: write response_format field: %w", err)
	}
	if err := writer.WriteField("stream", "false"); err != nil {
		return nil, fmt.Errorf("grok: write stream field: %w", err)
	}

	part, err := writer.CreateFormFile("image", "image.png")
	if err != nil {
		return nil, fmt.Errorf("grok: create form file: %w", err)
	}
	if _, err := part.Write(imageData); err != nil {
		return nil, fmt.Errorf("grok: write image data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("grok: close multipart writer: %w", err)
	}

	endpoint := c.baseURL + "/v1/images/edits"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("grok: create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("grok: read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("grok: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	imageURL, err := extractEditImageURL(respBody)
	if err != nil {
		return nil, err
	}

	resultData, err := downloadURL(ctx, imageURL)
	if err != nil {
		return nil, fmt.Errorf("grok: download edited image: %w", err)
	}

	if len(resultData) == 0 {
		return nil, fmt.Errorf("grok: downloaded edited image is empty")
	}

	return &ImageResult{ImageData: resultData}, nil
}

// GenerateVideo generates a video from text prompt, optionally with a reference image URL.
func (c *Client) GenerateVideo(ctx context.Context, prompt string, imageURL string) (*VideoResult, error) {
	content := []map[string]interface{}{
		{"type": "text", "text": prompt},
	}
	if imageURL != "" {
		content = append(content, map[string]interface{}{
			"type":      "image_url",
			"image_url": map[string]string{"url": imageURL},
		})
	}

	requestBody := map[string]interface{}{
		"model": c.videoModel,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": content,
			},
		},
		"video_config": map[string]interface{}{
			"aspect_ratio":    DefaultVideoRatio,
			"video_length":    DefaultVideoLen,
			"resolution_name": DefaultVideoRes,
			"preset":          DefaultVideoPreset,
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("grok: marshal request: %w", err)
	}

	endpoint := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("grok: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok: send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("grok: read response: %w", err)
	}

	if resp.StatusCode != 200 {
		payloads, _ := extractJSONPayloads(body)
		return nil, buildAPIError(body, payloads, resp.StatusCode)
	}

	payloads, err := extractJSONPayloads(body)
	if err != nil {
		return nil, err
	}
	if err := detectAPIError(payloads); err != nil {
		return nil, err
	}

	videoURL, err := extractVideoURLFromPayloads(payloads)
	if err != nil {
		return nil, err
	}

	videoData, err := downloadURL(ctx, videoURL)
	if err != nil {
		return nil, fmt.Errorf("grok: download video: %w", err)
	}

	if len(videoData) == 0 {
		return nil, fmt.Errorf("grok: downloaded video is empty")
	}

	return &VideoResult{VideoData: videoData}, nil
}

func extractJSONPayloads(body []byte) ([][]byte, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("grok: empty response body")
	}
	if json.Valid(trimmed) {
		return [][]byte{trimmed}, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	payloads := make([][]byte, 0, 4)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		if json.Valid([]byte(payload)) {
			payloads = append(payloads, []byte(payload))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("grok: read SSE response: %w", err)
	}
	if len(payloads) == 0 {
		return nil, fmt.Errorf("grok: no JSON payload found in response")
	}

	return payloads, nil
}

func buildAPIError(body []byte, payloads [][]byte, statusCode int) error {
	if msg := extractAPIErrorMessageFromPayloads(payloads); msg != "" {
		return fmt.Errorf("grok: API error (status %d): %s", statusCode, msg)
	}
	return fmt.Errorf("grok: API error (status %d): %s", statusCode, strings.TrimSpace(string(body)))
}

func detectAPIError(payloads [][]byte) error {
	if msg := extractAPIErrorMessageFromPayloads(payloads); msg != "" {
		return fmt.Errorf("grok: API error: %s", msg)
	}
	return nil
}

func extractAPIErrorMessageFromPayloads(payloads [][]byte) string {
	for i := len(payloads) - 1; i >= 0; i-- {
		if msg := extractAPIErrorMessage(payloads[i]); msg != "" {
			return msg
		}
	}
	return ""
}

func extractAPIErrorMessage(body []byte) string {
	var result struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return ""
	}
	return strings.TrimSpace(result.Error.Message)
}

// extractImageURL extracts the image URL from a chat completions response.
func extractImageURL(body []byte) (string, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("grok: parse response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		// Try data array (image generation response format)
		return extractEditImageURL(body)
	}

	choice := choices[0].(map[string]interface{})
	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("grok: no message in choice")
	}

	if imageURL := extractURLFromContentValue(message["content"]); imageURL != "" {
		return imageURL, nil
	}

	return "", fmt.Errorf("grok: no image URL found in response")
}

func extractImageURLFromPayloads(payloads [][]byte) (string, error) {
	var lastErr error
	for i := len(payloads) - 1; i >= 0; i-- {
		imageURL, err := extractImageURL(payloads[i])
		if err == nil {
			return imageURL, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("grok: no image URL found in response")
}

// extractEditImageURL extracts the image URL from an images/edits response (data array format).
func extractEditImageURL(body []byte) (string, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("grok: parse response: %w", err)
	}

	data, ok := result["data"].([]interface{})
	if !ok || len(data) == 0 {
		return "", fmt.Errorf("grok: no data in response")
	}

	first := data[0].(map[string]interface{})
	if u, ok := first["url"].(string); ok && u != "" {
		return u, nil
	}

	return "", fmt.Errorf("grok: no URL in data response")
}

func extractVideoURLFromPayloads(payloads [][]byte) (string, error) {
	var lastErr error
	for i := len(payloads) - 1; i >= 0; i-- {
		videoURL, err := extractVideoURL(payloads[i])
		if err == nil {
			return videoURL, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("grok: no video URL found in response")
}

// extractVideoURL extracts the video URL from a chat completions response.
func extractVideoURL(body []byte) (string, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("grok: parse response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("grok: no choices in response")
	}

	choice := choices[0].(map[string]interface{})
	if message, ok := choice["message"].(map[string]interface{}); ok {
		if videoURL := extractURLFromContentValue(message["content"]); videoURL != "" {
			return videoURL, nil
		}
	}
	if delta, ok := choice["delta"].(map[string]interface{}); ok {
		if videoURL := extractURLFromContentValue(delta["content"]); videoURL != "" {
			return videoURL, nil
		}
	}
	return "", fmt.Errorf("grok: no video URL found in response")
}

func extractURLFromContentValue(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return extractURLFromStringContent(typed)
	case []interface{}:
		for _, item := range typed {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if itemMap["type"] == "video_url" {
				if videoURL, ok := itemMap["video_url"].(map[string]interface{}); ok {
					if u, ok := videoURL["url"].(string); ok && u != "" {
						return u
					}
				}
			}
			if itemMap["type"] == "image_url" {
				if imageURL, ok := itemMap["image_url"].(map[string]interface{}); ok {
					if u, ok := imageURL["url"].(string); ok && u != "" {
						return u
					}
				}
			}
		}
	}
	return ""
}

func extractURLFromStringContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if matches := sourceAttrPattern.FindStringSubmatch(content); len(matches) == 2 {
		return matches[1]
	}
	if match := httpURLPattern.FindString(content); match != "" {
		return match
	}
	return ""
}

// downloadURL downloads content from a URL.
func downloadURL(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
}

type ImageResult struct {
	ImageData []byte
	Text      string
}

type TTSResult struct {
	AudioData []byte
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// GenerateImage 生成翻譯後的漫畫圖片
func (c *Client) GenerateImage(ctx context.Context, imageData []byte, mimeType, prompt, quality string) (*ImageResult, error) {
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	requestBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
					{
						"inline_data": map[string]string{
							"mime_type": mimeType,
							"data":      imageBase64,
						},
					},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"IMAGE"},
			"imageConfig": map[string]interface{}{
				"imageSize": quality,
			},
		},
		"safetySettings": []map[string]interface{}{
			{"category": "HARM_CATEGORY_HATE_SPEECH", "threshold": "OFF"},
			{"category": "HARM_CATEGORY_HARASSMENT", "threshold": "OFF"},
			{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT", "threshold": "OFF"},
			{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "OFF"},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-3-pro-image-preview:generateContent?key=%s", c.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	// 解析回應取得圖片
	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	candidate := candidates[0].(map[string]interface{})
	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no content in candidate")
	}

	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return nil, fmt.Errorf("no parts in content")
	}

	for _, part := range parts {
		partMap := part.(map[string]interface{})
		if inlineData, ok := partMap["inlineData"].(map[string]interface{}); ok {
			if dataStr, ok := inlineData["data"].(string); ok {
				imageBytes, err := base64.StdEncoding.DecodeString(dataStr)
				if err != nil {
					return nil, err
				}
				return &ImageResult{ImageData: imageBytes}, nil
			}
		}
	}

	return nil, fmt.Errorf("no image data in response")
}

// ExtractText 從圖片擷取文字
func (c *Client) ExtractText(ctx context.Context, imageData []byte, mimeType, prompt string) (string, error) {
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	requestBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
					{
						"inline_data": map[string]string{
							"mime_type": mimeType,
							"data":      imageBase64,
						},
					},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", c.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error: %s", string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	// 解析文字回應
	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return "", fmt.Errorf("no candidates in response")
	}

	candidate := candidates[0].(map[string]interface{})
	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no content in candidate")
	}

	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return "", fmt.Errorf("no parts in content")
	}

	for _, part := range parts {
		partMap := part.(map[string]interface{})
		if text, ok := partMap["text"].(string); ok {
			return text, nil
		}
	}

	return "", fmt.Errorf("no text in response")
}

// GenerateTTS 生成語音
func (c *Client) GenerateTTS(ctx context.Context, text, voiceName string) (*TTSResult, error) {
	requestBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": fmt.Sprintf("请用自然的语气朗读以下漫画对话内容：\n\n%s", text)},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"AUDIO"},
			"speechConfig": map[string]interface{}{
				"voiceConfig": map[string]interface{}{
					"prebuiltVoiceConfig": map[string]string{
						"voiceName": voiceName,
					},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-preview-tts:generateContent?key=%s", c.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	// 解析音訊回應
	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	candidate := candidates[0].(map[string]interface{})
	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no content in candidate")
	}

	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return nil, fmt.Errorf("no parts in content")
	}

	for _, part := range parts {
		partMap := part.(map[string]interface{})
		if inlineData, ok := partMap["inlineData"].(map[string]interface{}); ok {
			if dataStr, ok := inlineData["data"].(string); ok {
				audioBytes, err := base64.StdEncoding.DecodeString(dataStr)
				if err != nil {
					return nil, err
				}
				return &TTSResult{AudioData: audioBytes}, nil
			}
		}
	}

	return nil, fmt.Errorf("no audio data in response")
}

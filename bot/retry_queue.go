package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"tg-bawer/database"
	"tg-bawer/gemini"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type failedGenerationPayload struct {
	Prompt       string               `json:"prompt"`
	Quality      string               `json:"quality"`
	AspectRatio  string               `json:"aspect_ratio,omitempty"`
	ImageFileIDs []string             `json:"image_file_ids,omitempty"`
	Service      gemini.ServiceConfig `json:"service"`
}

func buildRetryQualities(quality string) []string {
	if quality == "" {
		quality = "2K"
	}
	return []string{quality, quality, quality, quality, quality, quality}
}

func (b *Bot) enqueueFailedGeneration(msg *tgbotapi.Message, replyToMessageID int, payload failedGenerationPayload, lastErr error) {
	if msg == nil || msg.From == nil {
		return
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		log.Printf("序列化失敗任務失敗: %v", err)
		return
	}

	lastError := ""
	if lastErr != nil {
		lastError = truncateError(lastErr.Error())
	}

	if err := b.db.AddFailedGeneration(msg.From.ID, msg.Chat.ID, int64(replyToMessageID), string(rawPayload), lastError); err != nil {
		log.Printf("寫入失敗任務失敗: %v", err)
	}
}

func (b *Bot) retryFailedGenerations() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		b.retryOneFailedGeneration()
	}
}

func (b *Bot) retryOneFailedGeneration() {
	task, err := b.db.GetRandomFailedGeneration()
	if err != nil {
		log.Printf("讀取失敗任務失敗: %v", err)
		return
	}
	if task == nil {
		return
	}

	var payload failedGenerationPayload
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		log.Printf("解析失敗任務 payload 失敗 (id=%d): %v", task.ID, err)
		b.db.DeleteFailedGeneration(task.ID)
		return
	}

	service := payload.Service
	if service.APIKey == "" {
		resolved, _, resolveErr := b.resolveServiceConfig(task.UserID)
		if resolveErr != nil {
			b.db.MarkFailedGenerationRetry(task.ID, resolveErr.Error())
			return
		}
		service = resolved
	}

	client := gemini.NewClientWithService(service)
	downloadedImages, err := b.downloadImagesByFileIDs(payload.ImageFileIDs)
	if err != nil {
		b.db.MarkFailedGenerationRetry(task.ID, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	aspectRatio := resolveAspectRatio(payload.AspectRatio, downloadedImages)

	var result *gemini.ImageResult
	if len(downloadedImages) > 0 {
		result, err = client.GenerateImageWithContext(ctx, downloadedImages, payload.Prompt, payload.Quality, aspectRatio)
	} else {
		result, err = client.GenerateImageFromText(ctx, payload.Prompt, payload.Quality, aspectRatio)
	}
	if err != nil {
		b.db.MarkFailedGenerationRetry(task.ID, err.Error())
		log.Printf("定時重試失敗 (id=%d): %v", task.ID, err)
		return
	}

	if err := b.sendRetrySuccessResult(task, payload, result); err != nil {
		b.db.MarkFailedGenerationRetry(task.ID, err.Error())
		log.Printf("定時重試成功但發送失敗 (id=%d): %v", task.ID, err)
		return
	}

	if err := b.db.DeleteFailedGeneration(task.ID); err != nil {
		log.Printf("刪除已成功重試任務失敗 (id=%d): %v", task.ID, err)
	}
}

func (b *Bot) downloadImagesByFileIDs(fileIDs []string) ([]gemini.DownloadedImage, error) {
	if len(fileIDs) == 0 {
		return nil, nil
	}

	downloadedImages := make([]gemini.DownloadedImage, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		file, err := b.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
		if err != nil {
			return nil, err
		}

		data, mimeType, err := b.downloadFile(file.FilePath)
		if err != nil {
			return nil, err
		}

		downloadedImages = append(downloadedImages, gemini.DownloadedImage{
			Data:     data,
			MimeType: mimeType,
		})
	}

	return downloadedImages, nil
}

func (b *Bot) sendRetrySuccessResult(task *database.FailedGeneration, payload failedGenerationPayload, result *gemini.ImageResult) error {
	if result == nil {
		return fmt.Errorf("empty retry result")
	}

	notice := tgbotapi.NewMessage(task.ChatID, fmt.Sprintf("♻️ 自動重試成功（任務 #%d）", task.ID))
	if task.ReplyToMessageID > 0 {
		notice.ReplyToMessageID = int(task.ReplyToMessageID)
	}
	if _, err := b.api.Send(notice); err != nil {
		return err
	}

	photoMsg := tgbotapi.NewPhoto(task.ChatID, tgbotapi.FileBytes{Name: "retry_preview.png", Bytes: result.ImageData})
	if task.ReplyToMessageID > 0 {
		photoMsg.ReplyToMessageID = int(task.ReplyToMessageID)
	}
	if _, err := b.api.Send(photoMsg); err != nil {
		return err
	}

	filename := "retry_generated.png"
	if payload.Quality != "" {
		filename = fmt.Sprintf("retry_generated_%s.png", payload.Quality)
	}
	docMsg := tgbotapi.NewDocument(task.ChatID, tgbotapi.FileBytes{Name: filename, Bytes: result.ImageData})
	docMsg.Caption = "📎 定時重試輸出（原畫質）"
	if task.ReplyToMessageID > 0 {
		docMsg.ReplyToMessageID = int(task.ReplyToMessageID)
	}
	if _, err := b.api.Send(docMsg); err != nil {
		return err
	}

	return nil
}

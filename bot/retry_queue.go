package bot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"tg-bawer/database"
	"tg-bawer/gemini"
	"tg-bawer/grok"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type failedGenerationPayload struct {
	Prompt           string               `json:"prompt"`
	Quality          string               `json:"quality"`
	AspectRatio      string               `json:"aspect_ratio,omitempty"`
	ImageFileIDs     []string             `json:"image_file_ids,omitempty"`
	Service          gemini.ServiceConfig `json:"service"`
	StatsOwnerUserID int64                `json:"stats_owner_user_id,omitempty"`
	StatsProvider    string               `json:"stats_provider,omitempty"`
	StatsModel       string               `json:"stats_model,omitempty"`
}

func statsTargetFromPayload(payload failedGenerationPayload) modelStatTarget {
	return modelStatTarget{
		OwnerUserID: payload.StatsOwnerUserID,
		Provider:    payload.StatsProvider,
		Model:       payload.StatsModel,
	}
}

func buildRetryQualities(quality string) []string {
	if quality == "" {
		quality = "2K"
	}
	return []string{quality, quality, quality, quality, quality, quality}
}

func (b *Bot) enqueueFailedGeneration(msg *tgbotapi.Message, replyToMessageID int, payload failedGenerationPayload, lastErr error, source string) {
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

	if source == "" {
		source = "google"
	}

	if err := b.db.AddFailedGeneration(msg.From.ID, msg.Chat.ID, int64(replyToMessageID), string(rawPayload), lastError, source); err != nil {
		log.Printf("寫入失敗任務失敗: %v", err)
	}
}

const (
	maxConcurrentRetries = 200
	normalRetryLimit     = 5
	vipRetryLimit        = 20
	adminRetryLimit      = 100
)

func isGoogleRateLimitErrorString(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, `"code":429`) ||
		strings.Contains(lower, "resource_exhausted") ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "quota")
}

func isGoogleRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	return isGoogleRateLimitErrorString(err.Error())
}

func (b *Bot) retryLimitForUser(userID int64) int {
	if b.isAdminUser(userID) {
		return adminRetryLimit
	}

	isVIP, err := b.db.IsVIPUser(userID)
	if err != nil {
		log.Printf("查詢 VIP 重試額度失敗 (userID=%d): %v", userID, err)
		return normalRetryLimit
	}
	if isVIP {
		return vipRetryLimit
	}
	return normalRetryLimit
}

func inferFailedTaskTarget(task *database.FailedGeneration, payload failedGenerationPayload) modelStatTarget {
	target := statsTargetFromPayload(payload)
	if target.OwnerUserID == 0 {
		target.OwnerUserID = task.UserID
	}
	if strings.TrimSpace(target.Provider) == "" {
		switch task.Source {
		case taskTypeGrokImage, taskTypeGrokVideo, "grok":
			target.Provider = "grok"
		default:
			target.Provider = "google"
		}
	}
	if strings.TrimSpace(target.Model) == "" {
		switch target.Provider {
		case "grok":
			switch task.Source {
			case taskTypeGrokVideo:
				target.Model = grok.DefaultVideoModel
			case taskTypeGrokImage:
				if len(payload.ImageFileIDs) > 0 {
					target.Model = grok.DefaultEditModel
				} else {
					target.Model = grok.DefaultImgModel
				}
			default:
				target.Model = grok.DefaultImgModel
			}
		default:
			target.Model = normalizeGoogleModel(payload.Service.Model)
		}
	}
	return target
}

func (b *Bot) cleanupFailedTaskResources(task *database.FailedGeneration, payload failedGenerationPayload) {
	for _, fileID := range payload.ImageFileIDs {
		if err := b.db.DecrementImageRefCountByFileID(task.UserID, task.ChatID, fileID); err != nil {
			log.Printf("警告：減少圖片引用計數失敗，可能導致資料庫殘留孤立記錄 (id=%d, file=%s): %v", task.ID, fileID, err)
		}
	}
}

func buildRetryExhaustedMessage(target modelStatTarget, prompt string, retryLimit int, lastError string) string {
	modelText := fmt.Sprintf("%s | %s", displayProviderName(target.Provider), normalizeModelName(target.Model))
	var sb strings.Builder
	sb.WriteString("❌ <b>自動重試最終失敗</b>\n\n")
	sb.WriteString("模型：<code>" + html.EscapeString(modelText) + "</code>\n")
	sb.WriteString(fmt.Sprintf("重試上限：%d 次", retryLimit))
	if prompt != "" {
		sb.WriteString("\n\n提示詞：\n<blockquote expandable>")
		sb.WriteString(html.EscapeString(prompt))
		sb.WriteString("</blockquote>")
	}
	if lastError != "" {
		sb.WriteString("\n\n最後錯誤：\n<blockquote expandable>")
		sb.WriteString(html.EscapeString(truncateError(lastError)))
		sb.WriteString("</blockquote>")
	}
	return sb.String()
}

func (b *Bot) notifyRetryExhausted(task *database.FailedGeneration, payload failedGenerationPayload, lastError string, retryLimit int) {
	text := buildRetryExhaustedMessage(inferFailedTaskTarget(task, payload), payload.Prompt, retryLimit, lastError)
	msg := tgbotapi.NewMessage(task.ChatID, text)
	msg.ParseMode = "HTML"
	if task.ReplyToMessageID > 0 {
		msg.ReplyToMessageID = int(task.ReplyToMessageID)
	}
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("發送最終失敗通知失敗 (id=%d): %v", task.ID, err)
	}
}

func (b *Bot) exhaustFailedGeneration(task *database.FailedGeneration, payload failedGenerationPayload, lastError string) {
	retryLimit := b.retryLimitForUser(task.UserID)
	b.recordModelFailure(inferFailedTaskTarget(task, payload))
	b.notifyRetryExhausted(task, payload, lastError, retryLimit)
	b.cleanupFailedTaskResources(task, payload)
	if err := b.db.DeleteFailedGeneration(task.ID); err != nil {
		log.Printf("刪除最終失敗任務失敗 (id=%d): %v", task.ID, err)
	}
}

func (b *Bot) markRetryFailure(task *database.FailedGeneration, payload failedGenerationPayload, lastError string) bool {
	nextRetryCount := task.RetryCount + 1
	retryLimit := b.retryLimitForUser(task.UserID)
	if nextRetryCount >= retryLimit {
		b.exhaustFailedGeneration(task, payload, lastError)
		return true
	}
	if err := b.db.MarkFailedGenerationRetry(task.ID, lastError); err != nil {
		log.Printf("標記重試失敗任務失敗 (id=%d): %v", task.ID, err)
	}
	return false
}

func (b *Bot) retryFailedGenerations() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if active, _ := b.maintenanceState(); active {
			continue
		}
		current := atomic.LoadInt32(&b.activeRetries)
		toSpawn := maxConcurrentRetries - int(current)
		if toSpawn <= 0 {
			continue
		}
		for i := 0; i < toSpawn; i++ {
			go b.retryOneFailedGeneration()
		}
	}
}

func (b *Bot) retryOneFailedGeneration() {
	if active, _ := b.maintenanceState(); active {
		return
	}

	task, err := b.db.GetRandomFailedGeneration()
	if err != nil {
		log.Printf("讀取失敗任務失敗: %v", err)
		return
	}
	if task == nil {
		return
	}

	atomic.AddInt32(&b.activeRetries, 1)
	defer atomic.AddInt32(&b.activeRetries, -1)

	var payload failedGenerationPayload
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		log.Printf("解析失敗任務 payload 失敗 (id=%d): %v", task.ID, err)
		b.db.DeleteFailedGeneration(task.ID)
		return
	}

	retryLimit := b.retryLimitForUser(task.UserID)
	if task.RetryCount >= retryLimit {
		log.Printf("任務達到最大重試次數 %d (id=%d)，刪除", retryLimit, task.ID)
		b.exhaustFailedGeneration(task, payload, task.LastError)
		return
	}

	// Normalise legacy source values to the current task type constants
	source := task.Source
	switch source {
	case "", "google":
		source = taskTypeGoogleImage
	case "grok":
		source = taskTypeGrokImage
	}

	// Handle Grok video retry separately
	if source == taskTypeGrokVideo {
		b.retryGrokVideoTask(task, payload)
		return
	}

	downloadedImages, err := b.downloadImagesByFileIDs(payload.ImageFileIDs)
	if err != nil {
		if b.markRetryFailure(task, payload, err.Error()) {
			return
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	aspectRatio := resolveAspectRatio(payload.AspectRatio, downloadedImages)

	var result *gemini.ImageResult
	resultSource := source // track which provider actually succeeded
	resultModel := ""      // track which model produced the result
	resultTarget := statsTargetFromPayload(payload)

	grokService := b.resolveGrokService(task.UserID)
	if source == taskTypeGrokImage && grokService != nil {
		// Grok image retry
		gc := grokService.Client
		modelName := gc.ImageModel()
		if len(downloadedImages) > 0 {
			modelName = gc.EditModel()
		}
		for attempt := 0; attempt < 6; attempt++ {
			var grokResult *grok.ImageResult
			if len(downloadedImages) > 0 {
				grokResult, err = gc.EditImage(ctx, downloadedImages[0].Data, payload.Prompt, "1024x1024")
			} else {
				grokResult, err = gc.GenerateImage(ctx, payload.Prompt, "1024x1024")
			}
			if err == nil && grokResult != nil && len(grokResult.ImageData) > 0 {
				result = &gemini.ImageResult{ImageData: grokResult.ImageData}
				resultTarget = buildGrokStatTarget(grokService.OwnerUserID, modelName)
				if len(downloadedImages) > 0 {
					resultModel = gc.EditModel()
				} else {
					resultModel = gc.ImageModel()
				}
				break
			}
			log.Printf("Grok retry attempt %d failed (id=%d): %v", attempt+1, task.ID, err)
			b.addErrorLog("Grok 圖片重試",
				fmt.Sprintf("task_id=%d, attempt=%d, prompt=%q, size=1024x1024, images=%d", task.ID, attempt+1, payload.Prompt, len(downloadedImages)),
				fmt.Sprintf("%v", err))
			time.Sleep(time.Second * 2)
		}
	} else {
		// Google image retry: try all user services
		allServices, _ := b.resolveAllServiceConfigs(task.UserID)
		if len(allServices) == 0 {
			// Fallback to payload service
			if payload.Service.APIKey != "" {
				allServices = append(allServices, resolvedGoogleService{
					OwnerUserID: payload.StatsOwnerUserID,
					Config:      payload.Service,
				})
			}
		}
		googleAll429 := len(allServices) > 0
		googleAttempted := false

		for _, svcCfg := range allServices {
			client := gemini.NewClientWithService(svcCfg.Config)
			modelName := normalizeGoogleModel(svcCfg.Config.Model)
			for attempt := 0; attempt < 6; attempt++ {
				googleAttempted = true
				if len(downloadedImages) > 0 {
					result, err = client.GenerateImageWithContext(ctx, downloadedImages, payload.Prompt, payload.Quality, aspectRatio)
				} else {
					result, err = client.GenerateImageFromText(ctx, payload.Prompt, payload.Quality, aspectRatio)
				}
				if err == nil && result != nil && len(result.ImageData) > 0 {
					googleAll429 = false
					break
				}
				if !isGoogleRateLimitError(err) {
					googleAll429 = false
				}
				log.Printf("Google retry service %s attempt %d failed (id=%d): %v", svcCfg.Config.Name, attempt+1, task.ID, err)
				b.addErrorLog("Google 圖片重試",
					fmt.Sprintf("task_id=%d, service=%s, attempt=%d, prompt=%q, quality=%s, aspect_ratio=%s, images=%d", task.ID, svcCfg.Config.Name, attempt+1, payload.Prompt, payload.Quality, aspectRatio, len(downloadedImages)),
					fmt.Sprintf("%v", err))
				time.Sleep(time.Second * 2)
			}
			if result != nil && len(result.ImageData) > 0 {
				resultSource = taskTypeGoogleImage
				resultTarget = buildGoogleStatTarget(svcCfg.OwnerUserID, modelName)
				resultModel = svcCfg.Config.Model
				if resultModel == "" {
					resultModel = gemini.DefaultImageModel
				}
				break
			}
		}

		if result == nil && googleAttempted && googleAll429 {
			errMsg := "google rate limited"
			if err != nil {
				errMsg = err.Error()
			}
			if updateErr := b.db.UpdateFailedGenerationRetryError(task.ID, errMsg); updateErr != nil {
				log.Printf("更新 429 重試錯誤失敗 (id=%d): %v", task.ID, updateErr)
			}
			log.Printf("Google 全部請求均為 429，不扣除重試次數 (id=%d)", task.ID)
			return
		}
	}

	if result == nil || len(result.ImageData) == 0 {
		errMsg := "unknown error"
		if err != nil {
			errMsg = err.Error()
		}
		if b.markRetryFailure(task, payload, errMsg) {
			return
		}
		log.Printf("定時重試失敗 (id=%d): %v", task.ID, err)
		b.addErrorLog("圖片重試最終失敗",
			fmt.Sprintf("task_id=%d, prompt=%q, quality=%s", task.ID, payload.Prompt, payload.Quality),
			fmt.Sprintf("%v", err))
		return
	}

	if err := b.sendRetrySuccessResult(task, payload, result, resultSource, resultModel); err != nil {
		if b.markRetryFailure(task, payload, err.Error()) {
			return
		}
		log.Printf("定時重試成功但發送失敗 (id=%d): %v", task.ID, err)
		return
	}
	b.recordModelSuccess(resultTarget)

	if err := b.db.DeleteFailedGeneration(task.ID); err != nil {
		log.Printf("刪除已成功重試任務失敗 (id=%d): %v", task.ID, err)
	}
}

// retryGrokVideoTask handles retry for a grok_video type failed task.
func (b *Bot) retryGrokVideoTask(task *database.FailedGeneration, payload failedGenerationPayload) {
	grokService := b.resolveGrokService(task.UserID)
	if grokService == nil {
		if b.markRetryFailure(task, payload, "Grok not available") {
			return
		}
		return
	}
	gc := grokService.Client

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Build image URL from stored file IDs if available
	imageURL := ""
	downloadedImages, dlErr := b.downloadImagesByFileIDs(payload.ImageFileIDs)
	if dlErr != nil {
		log.Printf("下載重試影片原圖失敗 (id=%d): %v", task.ID, dlErr)
	}
	if dlErr == nil && len(downloadedImages) > 0 {
		imageURL = "data:" + downloadedImages[0].MimeType + ";base64," + base64.StdEncoding.EncodeToString(downloadedImages[0].Data)
	}

	var videoResult *grok.VideoResult
	var lastErr error
	modelName := gc.VideoModel()
	for attempt := 0; attempt < 6; attempt++ {
		videoResult, lastErr = gc.GenerateVideo(ctx, payload.Prompt, imageURL)
		if lastErr == nil && videoResult != nil && len(videoResult.VideoData) > 0 {
			break
		}
		log.Printf("Grok video retry attempt %d failed (id=%d): %v", attempt+1, task.ID, lastErr)
		b.addErrorLog("Grok 影片重試",
			fmt.Sprintf("task_id=%d, attempt=%d, prompt=%q", task.ID, attempt+1, payload.Prompt),
			fmt.Sprintf("%v", lastErr))
		time.Sleep(time.Second * 2)
	}

	if videoResult == nil || len(videoResult.VideoData) == 0 {
		errMsg := "unknown error"
		if lastErr != nil {
			errMsg = lastErr.Error()
		}
		if b.markRetryFailure(task, payload, errMsg) {
			return
		}
		log.Printf("定時重試影片失敗 (id=%d): %v", task.ID, lastErr)
		b.addErrorLog("Grok 影片重試最終失敗",
			fmt.Sprintf("task_id=%d, prompt=%q", task.ID, payload.Prompt),
			fmt.Sprintf("%v", lastErr))
		return
	}

	notice := tgbotapi.NewMessage(task.ChatID, fmt.Sprintf("♻️ 自動重試成功（影片任務 #%d）", task.ID))
	if task.ReplyToMessageID > 0 {
		notice.ReplyToMessageID = int(task.ReplyToMessageID)
	}
	b.api.Send(notice)

	label := "🎬 定時重試影片" + generationTypeLabel(len(payload.ImageFileIDs) > 0, "video")
	videoMsg := tgbotapi.NewVideo(task.ChatID, tgbotapi.FileBytes{Name: "retry_generated.mp4", Bytes: videoResult.VideoData})
	if task.ReplyToMessageID > 0 {
		videoMsg.ReplyToMessageID = int(task.ReplyToMessageID)
	}
	videoMsg.Caption = buildCaptionForResult(label, payload.Prompt)
	videoMsg.ParseMode = "HTML"
	if sentMsg, err := b.api.Send(videoMsg); err == nil {
		b.db.SaveBotReplyPrompt(task.ChatID, sentMsg.MessageID, payload.Prompt, label, "video")
		b.sendFollowUpMessages(task.ChatID, sentMsg.MessageID, payload.Prompt, payload.ImageFileIDs)
	}

	if err := b.db.DeleteFailedGeneration(task.ID); err != nil {
		log.Printf("刪除已成功重試影片任務失敗 (id=%d): %v", task.ID, err)
	}
	b.recordModelSuccess(buildGrokStatTarget(grokService.OwnerUserID, modelName))
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

func (b *Bot) sendRetrySuccessResult(task *database.FailedGeneration, payload failedGenerationPayload, result *gemini.ImageResult, resultSource string, resultModel string) error {
	if result == nil || len(result.ImageData) == 0 {
		return fmt.Errorf("empty retry result")
	}

	// Build a human-readable label for the result source
	var sourceLabel string
	hasImages := len(payload.ImageFileIDs) > 0
	switch resultSource {
	case taskTypeGrokImage:
		sourceLabel = "🎭 Grok 圖片" + generationTypeLabel(hasImages, "image")
	case taskTypeGoogleImage:
		sourceLabel = "🌐 Google 圖片" + generationTypeLabel(hasImages, "image")
	default:
		sourceLabel = "🖼 圖片"
		log.Printf("sendRetrySuccessResult: unexpected resultSource=%q (task #%d)", resultSource, task.ID)
	}
	if resultModel != "" {
		sourceLabel += " - " + resultModel
	}

	notice := tgbotapi.NewMessage(task.ChatID, fmt.Sprintf("♻️ 自動重試成功（任務 #%d）\n結果來源：%s", task.ID, sourceLabel))
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
	photoMsg.Caption = buildCaptionForResult(sourceLabel, payload.Prompt)
	photoMsg.ParseMode = "HTML"
	sentPhoto, err := b.api.Send(photoMsg)
	if err != nil {
		return fmt.Errorf("發送預覽圖失敗: %w", err)
	}
	// Verify the photo was actually uploaded
	if len(sentPhoto.Photo) == 0 {
		return fmt.Errorf("預覽圖上傳失敗：未收到確認")
	}
	b.db.SaveBotReplyPrompt(task.ChatID, sentPhoto.MessageID, payload.Prompt, sourceLabel, "photo")
	b.sendFollowUpMessages(task.ChatID, sentPhoto.MessageID, payload.Prompt, payload.ImageFileIDs)

	filename := "retry_generated.png"
	if payload.Quality != "" {
		filename = fmt.Sprintf("retry_generated_%s.png", payload.Quality)
	}
	docMsg := tgbotapi.NewDocument(task.ChatID, tgbotapi.FileBytes{Name: filename, Bytes: result.ImageData})
	docMsg.Caption = fmt.Sprintf("📎 定時重試輸出（原畫質）｜%s", sourceLabel)
	if task.ReplyToMessageID > 0 {
		docMsg.ReplyToMessageID = int(task.ReplyToMessageID)
	}
	sentDoc, err := b.api.Send(docMsg)
	if err != nil {
		return fmt.Errorf("發送原檔案失敗: %w", err)
	}
	// Verify the document was actually uploaded
	if sentDoc.Document == nil {
		return fmt.Errorf("原檔案上傳失敗：未收到確認")
	}

	return nil
}

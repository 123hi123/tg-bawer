package bot

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"tg-bawer/gemini"
	"tg-bawer/grok"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	taskTypeGoogleImage = "google_image"
	taskTypeGrokImage   = "grok_image"
	taskTypeGrokVideo   = "grok_video"
)

// runAllGenerationTasks launches Google image, Grok image, and Grok video generation
// tasks concurrently. Each task sends its result (or queues a failure) independently.
// The status message is deleted or updated after all tasks complete.
func (b *Bot) runAllGenerationTasks(
	msg *tgbotapi.Message,
	replyToMsgID int,
	prompt, quality, aspectRatio string,
	downloadedImages []gemini.DownloadedImage,
	imageFileIDs []string,
	allServices []resolvedGoogleService,
	extraGoogleModels []string,
	statusMsgID int,
) {
	var wg sync.WaitGroup

	// Count expected results: main Google task + extra model tasks + Grok image task.
	// Grok video is treated as optional/best-effort and does not report into this
	// channel; its failures are silently enqueued for retry via /queue.
	taskCount := 0
	if len(allServices) > 0 {
		taskCount++ // main Google task
		taskCount += len(extraGoogleModels)
	}
	grokService := b.resolveGrokService(msg.From.ID)
	if grokService != nil {
		taskCount++
	}
	resultCh := make(chan bool, taskCount)

	// Update status message with task summary
	if statusMsgID > 0 && taskCount > 0 {
		var gc *grok.Client
		if grokService != nil {
			gc = grokService.Client
		}
		summary := buildTaskSummary(allServices, extraGoogleModels, gc, len(downloadedImages) > 0)
		if summary != "" {
			statusText := "⏳ *同時生成中...*" + summary
			edit := tgbotapi.NewEditMessageText(msg.Chat.ID, statusMsgID, statusText)
			edit.ParseMode = "Markdown"
			b.api.Send(edit)
		}
	}

	// Google image task (main model)
	if len(allServices) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resultCh <- b.runGoogleImageTask(msg, replyToMsgID, prompt, quality, aspectRatio, downloadedImages, imageFileIDs, allServices, "", "")
		}()

		// Extra model tasks for Google channel
		for _, extraModel := range extraGoogleModels {
			extraModel := extraModel // capture loop variable
			wg.Add(1)
			go func() {
				defer wg.Done()
				resultCh <- b.runGoogleImageTask(msg, replyToMsgID, prompt, quality, aspectRatio, downloadedImages, imageFileIDs, allServices, extraModel, "")
			}()
		}
	}

	// Grok image and video tasks
	if grokService != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resultCh <- b.runGrokImageTask(grokService, msg, replyToMsgID, prompt, downloadedImages, imageFileIDs)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			b.runGrokVideoTask(grokService, msg, replyToMsgID, prompt, downloadedImages, imageFileIDs)
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)

		failedCount := 0
		for ok := range resultCh {
			if !ok {
				failedCount++
			}
		}

		if statusMsgID > 0 {
			if failedCount == 0 {
				b.api.Request(tgbotapi.NewDeleteMessage(msg.Chat.ID, statusMsgID))
			} else {
				edit := tgbotapi.NewEditMessageText(msg.Chat.ID, statusMsgID,
					fmt.Sprintf("⚠️ %d 個子任務失敗，已加入重試佇列。\n用 /queue 查看詳情。", failedCount))
				b.api.Send(edit)
			}
		}
	}()
}

// runGoogleImageTask runs Google image generation with all available services.
// modelOverride overrides the model for all services (empty = use each service's default model).
// label is used as the caption prefix (empty = auto-generated from modelOverride or default).
// Returns true on success, false on failure (failure is enqueued for retry).
func (b *Bot) runGoogleImageTask(
	msg *tgbotapi.Message,
	replyToMsgID int,
	prompt, quality, aspectRatio string,
	downloadedImages []gemini.DownloadedImage,
	imageFileIDs []string,
	allServices []resolvedGoogleService,
	modelOverride string,
	label string,
) bool {
	// Build caption label
	if label == "" {
		if modelOverride != "" {
			label = "⚡ Google Flash 圖片"
		} else {
			label = "🌐 Google 圖片"
		}
	}
	label += generationTypeLabel(len(downloadedImages) > 0, "image")

	// Override model in service configs if requested
	services := allServices
	if modelOverride != "" {
		overridden := make([]resolvedGoogleService, len(allServices))
		for i, svc := range allServices {
			svc.Config.Model = modelOverride
			overridden[i] = svc
		}
		services = overridden
	}
	ctx := context.Background()
	var result *gemini.ImageResult
	var lastErr error
	defaultTarget := modelStatTarget{}
	if len(services) > 0 {
		defaultTarget = buildGoogleStatTarget(services[0].OwnerUserID, services[0].Config.Model)
		b.recordModelUsage(defaultTarget)
	}

	for _, svcCfg := range services {
		gClient := gemini.NewClientWithService(svcCfg.Config)
		modelName := normalizeGoogleModel(svcCfg.Config.Model)
		for attempt := 0; attempt < 6; attempt++ {
			if len(downloadedImages) > 0 {
				result, lastErr = gClient.GenerateImageWithContext(ctx, downloadedImages, prompt, quality, aspectRatio)
			} else {
				result, lastErr = gClient.GenerateImageFromText(ctx, prompt, quality, aspectRatio)
			}
			if lastErr == nil && result != nil && len(result.ImageData) > 0 {
				b.recordModelSuccess(buildGoogleStatTarget(svcCfg.OwnerUserID, modelName))
				break
			}
			log.Printf("Google service %s attempt %d failed: %v", svcCfg.Config.Name, attempt+1, lastErr)
			b.addErrorLog("Google 圖片",
				fmt.Sprintf("service=%s, attempt=%d, prompt=%q, quality=%s, aspect_ratio=%s, images=%d", svcCfg.Config.Name, attempt+1, prompt, quality, aspectRatio, len(downloadedImages)),
				fmt.Sprintf("%v", lastErr))
			time.Sleep(2 * time.Second)
		}
		if result != nil && len(result.ImageData) > 0 {
			break
		}
	}

	if result == nil || len(result.ImageData) == 0 {
		var svcCfg resolvedGoogleService
		if len(services) > 0 {
			svcCfg = services[0]
		}
		b.enqueueFailedGeneration(msg, replyToMsgID, failedGenerationPayload{
			Prompt:           prompt,
			Quality:          quality,
			AspectRatio:      aspectRatio,
			ImageFileIDs:     imageFileIDs,
			Service:          svcCfg.Config,
			StatsOwnerUserID: defaultTarget.OwnerUserID,
			StatsProvider:    defaultTarget.Provider,
			StatsModel:       defaultTarget.Model,
		}, lastErr, taskTypeGoogleImage)
		log.Printf("Google 圖片生成失敗，已加入重試佇列")
		return false
	}

	// Send compressed preview
	photoMsg := tgbotapi.NewPhoto(msg.Chat.ID, tgbotapi.FileBytes{Name: "google_preview.png", Bytes: result.ImageData})
	photoMsg.ReplyToMessageID = replyToMsgID
	photoMsg.Caption = buildCaptionForResult(label, prompt)
	photoMsg.ParseMode = "HTML"
	if sentMsg, err := b.api.Send(photoMsg); err != nil {
		log.Printf("發送 Google 預覽圖失敗: %v", err)
	} else {
		b.db.SaveBotReplyPrompt(msg.Chat.ID, sentMsg.MessageID, prompt, label, "photo")
		b.sendFollowUpMessages(msg.Chat.ID, sentMsg.MessageID, prompt, imageFileIDs)
	}

	// Send full-quality document for 2K/4K
	if quality == "4K" || quality == "2K" {
		docMsg := tgbotapi.NewDocument(msg.Chat.ID, tgbotapi.FileBytes{
			Name:  fmt.Sprintf("google_generated_%s.png", quality),
			Bytes: result.ImageData,
		})
		docMsg.ReplyToMessageID = replyToMsgID
		docMsg.Caption = "📎 Google 原畫質"
		if _, err := b.api.Send(docMsg); err != nil {
			log.Printf("發送 Google 原檔案失敗: %v", err)
		}
	}

	return true
}

// runGrokImageTask runs Grok image generation (or editing if images are provided).
// Returns true on success, false on failure (failure is enqueued for retry).
func (b *Bot) runGrokImageTask(
	grokService *resolvedGrokService,
	msg *tgbotapi.Message,
	replyToMsgID int,
	prompt string,
	downloadedImages []gemini.DownloadedImage,
	imageFileIDs []string,
) bool {
	gc := grokService.Client
	ctx := context.Background()
	var result *grok.ImageResult
	var lastErr error
	modelName := gc.ImageModel()
	if len(downloadedImages) > 0 {
		modelName = gc.EditModel()
	}
	target := buildGrokStatTarget(grokService.OwnerUserID, modelName)
	b.recordModelUsage(target)

	for attempt := 0; attempt < 6; attempt++ {
		if len(downloadedImages) > 0 {
			result, lastErr = gc.EditImage(ctx, downloadedImages[0].Data, prompt, "1024x1024")
		} else {
			result, lastErr = gc.GenerateImage(ctx, prompt, "1024x1024")
		}
		if lastErr == nil && result != nil && len(result.ImageData) > 0 {
			b.recordModelSuccess(target)
			break
		}
		log.Printf("Grok image attempt %d failed: %v", attempt+1, lastErr)
		b.addErrorLog("Grok 圖片",
			fmt.Sprintf("attempt=%d, prompt=%q, size=1024x1024, images=%d", attempt+1, prompt, len(downloadedImages)),
			fmt.Sprintf("%v", lastErr))
		time.Sleep(2 * time.Second)
	}

	if result == nil || len(result.ImageData) == 0 {
		b.enqueueFailedGeneration(msg, replyToMsgID, failedGenerationPayload{
			Prompt:           prompt,
			ImageFileIDs:     imageFileIDs,
			StatsOwnerUserID: target.OwnerUserID,
			StatsProvider:    target.Provider,
			StatsModel:       target.Model,
		}, lastErr, taskTypeGrokImage)
		log.Printf("Grok 圖片生成失敗，已加入重試佇列")
		return false
	}

	photoMsg := tgbotapi.NewPhoto(msg.Chat.ID, tgbotapi.FileBytes{Name: "grok_preview.png", Bytes: result.ImageData})
	photoMsg.ReplyToMessageID = replyToMsgID
	label := "🎭 Grok 圖片" + generationTypeLabel(len(downloadedImages) > 0, "image")
	photoMsg.Caption = buildCaptionForResult(label, prompt)
	photoMsg.ParseMode = "HTML"
	if sentMsg, err := b.api.Send(photoMsg); err != nil {
		log.Printf("發送 Grok 預覽圖失敗: %v", err)
	} else {
		b.db.SaveBotReplyPrompt(msg.Chat.ID, sentMsg.MessageID, prompt, label, "photo")
		b.sendFollowUpMessages(msg.Chat.ID, sentMsg.MessageID, prompt, imageFileIDs)
	}

	return true
}

// runGrokVideoTask runs Grok video generation and uploads the result.
// Failures are silently enqueued for retry (no error message sent to user).
func (b *Bot) runGrokVideoTask(
	grokService *resolvedGrokService,
	msg *tgbotapi.Message,
	replyToMsgID int,
	prompt string,
	downloadedImages []gemini.DownloadedImage,
	imageFileIDs []string,
) {
	gc := grokService.Client
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Build image URL from first downloaded image if available.
	// The Grok video API accepts a single reference image; imageFileIDs is kept
	// for labeling (圖生影片) and for sending original images as follow-up replies.
	imageURL := ""
	if len(downloadedImages) > 0 {
		imageURL = "data:" + downloadedImages[0].MimeType + ";base64," + base64.StdEncoding.EncodeToString(downloadedImages[0].Data)
	}

	var result *grok.VideoResult
	var lastErr error
	modelName := gc.VideoModel()
	target := buildGrokStatTarget(grokService.OwnerUserID, modelName)
	b.recordModelUsage(target)

	for attempt := 0; attempt < 6; attempt++ {
		result, lastErr = gc.GenerateVideo(ctx, prompt, imageURL)
		if lastErr == nil && result != nil && len(result.VideoData) > 0 {
			b.recordModelSuccess(target)
			break
		}
		log.Printf("Grok video attempt %d failed: %v", attempt+1, lastErr)
		b.addErrorLog("Grok 影片",
			fmt.Sprintf("attempt=%d, prompt=%q", attempt+1, prompt),
			fmt.Sprintf("%v", lastErr))
		time.Sleep(2 * time.Second)
	}

	if result == nil || len(result.VideoData) == 0 {
		b.enqueueFailedGeneration(msg, replyToMsgID, failedGenerationPayload{
			Prompt:           prompt,
			ImageFileIDs:     imageFileIDs,
			StatsOwnerUserID: target.OwnerUserID,
			StatsProvider:    target.Provider,
			StatsModel:       target.Model,
		}, lastErr, taskTypeGrokVideo)
		log.Printf("Grok 影片生成失敗，已加入重試佇列")
		return
	}

	label := "🎬 Grok 影片" + generationTypeLabel(len(downloadedImages) > 0, "video")
	videoMsg := tgbotapi.NewVideo(msg.Chat.ID, tgbotapi.FileBytes{Name: "generated.mp4", Bytes: result.VideoData})
	videoMsg.ReplyToMessageID = replyToMsgID
	videoMsg.Caption = buildCaptionForResult(label, prompt)
	videoMsg.ParseMode = "HTML"
	if sentMsg, err := b.api.Send(videoMsg); err != nil {
		log.Printf("上傳影片失敗: %v", err)
	} else {
		b.db.SaveBotReplyPrompt(msg.Chat.ID, sentMsg.MessageID, prompt, label, "video")
		b.sendFollowUpMessages(msg.Chat.ID, sentMsg.MessageID, prompt, imageFileIDs)
	}
}

// buildTaskSummary builds a Markdown summary of all generation tasks that will run.
func buildTaskSummary(allServices []resolvedGoogleService, extraGoogleModels []string, gc *grok.Client, hasImages bool) string {
	var lines []string

	if len(allServices) > 0 {
		model := allServices[0].Config.Model
		if model == "" {
			model = gemini.DefaultImageModel
		}
		lines = append(lines, fmt.Sprintf("• 🌐 Google `%s` 正在繪製", model))

		for _, extraModel := range extraGoogleModels {
			lines = append(lines, fmt.Sprintf("• ⚡ Google `%s` 正在繪製", extraModel))
		}
	}

	if gc != nil {
		imgModel := gc.ImageModel()
		if hasImages {
			imgModel = gc.EditModel()
		}
		lines = append(lines, fmt.Sprintf("• 🎭 Grok `%s` 正在繪製", imgModel))
		lines = append(lines, fmt.Sprintf("• 🎬 Grok `%s` 正在製作", gc.VideoModel()))
	}

	if len(lines) == 0 {
		return ""
	}

	return fmt.Sprintf("\n\n🔄 *任務列表（共 %d 個服務）：*\n%s", len(lines), strings.Join(lines, "\n"))
}

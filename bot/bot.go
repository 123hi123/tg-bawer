package bot

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"gemini-manga-bot/config"
	"gemini-manga-bot/database"
	"gemini-manga-bot/gemini"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api      *tgbotapi.BotAPI
	gemini   *gemini.Client
	db       *database.Database
	config   *config.Config
}

func NewBot(cfg *config.Config, db *database.Database) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}

	log.Printf("Bot authorized on account %s", api.Self.UserName)

	return &Bot{
		api:    api,
		gemini: gemini.NewClient(cfg.GeminiAPIKey),
		db:     db,
		config: cfg,
	}, nil
}

func (b *Bot) Run() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			go b.handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			go b.handleCallback(update.CallbackQuery)
		}
	}
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	// 處理指令
	if msg.IsCommand() {
		b.handleCommand(msg)
		return
	}

	// 處理圖片
	if msg.Photo != nil && len(msg.Photo) > 0 {
		b.handlePhoto(msg)
		return
	}
}

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		b.cmdStart(msg)
	case "help":
		b.cmdHelp(msg)
	case "save":
		b.cmdSave(msg)
	case "list":
		b.cmdList(msg)
	case "history":
		b.cmdHistory(msg)
	case "setdefault":
		b.cmdSetDefault(msg)
	case "settings":
		b.cmdSettings(msg)
	case "delete":
		b.cmdDelete(msg)
	}
}

func (b *Bot) cmdStart(msg *tgbotapi.Message) {
	text := `🎨 *Gemini 漫畫翻譯 Bot*

歡迎使用！直接傳送漫畫圖片即可自動翻譯。

*基本用法：*
• 直接傳圖片 → 使用預設 Prompt 翻譯
• 圖片 + 文字 → 使用該文字作為 Prompt

*圖片參數（在圖片說明中使用）：*
• ` + "`/s 4K`" + ` → 設定畫質（1K/2K/4K）
• ` + "`/v`" + ` → 同時生成語音朗讀

*指令：*
/save <名稱> <prompt> - 保存 Prompt
/list - 列出已保存的 Prompt
/history - 查看使用歷史
/setdefault - 設定預設 Prompt
/settings - 設定預設畫質
/help - 顯示幫助`

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

func (b *Bot) cmdHelp(msg *tgbotapi.Message) {
	b.cmdStart(msg)
}

func (b *Bot) cmdSave(msg *tgbotapi.Message) {
	args := msg.CommandArguments()
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ 格式：/save <名稱> <prompt>\n例如：/save 學習模式 漫画的文本翻譯为中文...")
		b.api.Send(reply)
		return
	}

	name := parts[0]
	prompt := parts[1]

	if err := b.db.SavePrompt(msg.From.ID, name, prompt); err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ 保存失敗："+err.Error())
		b.api.Send(reply)
		return
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ 已保存 Prompt「%s」", name))
	b.api.Send(reply)
}

func (b *Bot) cmdList(msg *tgbotapi.Message) {
	prompts, err := b.db.GetSavedPrompts(msg.From.ID)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ 取得失敗："+err.Error())
		b.api.Send(reply)
		return
	}

	if len(prompts) == 0 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "📝 尚未保存任何 Prompt\n使用 /save <名稱> <prompt> 來保存")
		b.api.Send(reply)
		return
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, p := range prompts {
		defaultMark := ""
		if p.IsDefault {
			defaultMark = " ⭐"
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%s%s", p.Name, defaultMark),
			fmt.Sprintf("copy:%d", p.ID),
		)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	reply := tgbotapi.NewMessage(msg.Chat.ID, "📋 *已保存的 Prompt*\n點擊可複製內容：")
	reply.ParseMode = "Markdown"
	reply.ReplyMarkup = keyboard
	b.api.Send(reply)
}

func (b *Bot) cmdHistory(msg *tgbotapi.Message) {
	history, err := b.db.GetHistory(msg.From.ID, 10)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ 取得失敗："+err.Error())
		b.api.Send(reply)
		return
	}

	if len(history) == 0 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "📜 尚無使用記錄")
		b.api.Send(reply)
		return
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for i, h := range history {
		preview := h.Prompt
		if len(preview) > 30 {
			preview = preview[:30] + "..."
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%d. %s", i+1, preview),
			fmt.Sprintf("hist:%d", h.ID),
		)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	reply := tgbotapi.NewMessage(msg.Chat.ID, "📜 *最近使用的 Prompt*\n點擊可複製：")
	reply.ParseMode = "Markdown"
	reply.ReplyMarkup = keyboard
	b.api.Send(reply)
}

func (b *Bot) cmdSetDefault(msg *tgbotapi.Message) {
	prompts, err := b.db.GetSavedPrompts(msg.From.ID)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ 取得失敗："+err.Error())
		b.api.Send(reply)
		return
	}

	if len(prompts) == 0 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "📝 尚未保存任何 Prompt\n先使用 /save 保存後再設定預設")
		b.api.Send(reply)
		return
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, p := range prompts {
		mark := "○"
		if p.IsDefault {
			mark = "●"
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%s %s", mark, p.Name),
			fmt.Sprintf("default:%d", p.ID),
		)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	reply := tgbotapi.NewMessage(msg.Chat.ID, "⭐ *選擇預設 Prompt*：")
	reply.ParseMode = "Markdown"
	reply.ReplyMarkup = keyboard
	b.api.Send(reply)
}

func (b *Bot) cmdSettings(msg *tgbotapi.Message) {
	currentQuality, _ := b.db.GetUserSettings(msg.From.ID)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(qualityButton("1K", currentQuality), "quality:1K"),
			tgbotapi.NewInlineKeyboardButtonData(qualityButton("2K", currentQuality), "quality:2K"),
			tgbotapi.NewInlineKeyboardButtonData(qualityButton("4K", currentQuality), "quality:4K"),
		),
	)

	reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("⚙️ *設定*\n\n目前預設畫質：*%s*\n\n點擊更改：", currentQuality))
	reply.ParseMode = "Markdown"
	reply.ReplyMarkup = keyboard
	b.api.Send(reply)
}

func qualityButton(q, current string) string {
	if q == current {
		return "● " + q
	}
	return "○ " + q
}

func (b *Bot) cmdDelete(msg *tgbotapi.Message) {
	prompts, err := b.db.GetSavedPrompts(msg.From.ID)
	if err != nil || len(prompts) == 0 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "📝 沒有可刪除的 Prompt")
		b.api.Send(reply)
		return
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, p := range prompts {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("🗑 %s", p.Name),
			fmt.Sprintf("del:%d", p.ID),
		)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	reply := tgbotapi.NewMessage(msg.Chat.ID, "🗑 *選擇要刪除的 Prompt*：")
	reply.ParseMode = "Markdown"
	reply.ReplyMarkup = keyboard
	b.api.Send(reply)
}

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	data := callback.Data
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return
	}

	action := parts[0]
	value := parts[1]

	switch action {
	case "copy":
		b.callbackCopy(callback, value)
	case "hist":
		b.callbackHistory(callback, value)
	case "default":
		b.callbackDefault(callback, value)
	case "quality":
		b.callbackQuality(callback, value)
	case "del":
		b.callbackDelete(callback, value)
	}
}

func (b *Bot) callbackCopy(callback *tgbotapi.CallbackQuery, idStr string) {
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	prompts, _ := b.db.GetSavedPrompts(callback.From.ID)
	for _, p := range prompts {
		if p.ID == id {
			// 發送 Prompt 內容讓使用者複製
			reply := tgbotapi.NewMessage(callback.Message.Chat.ID, fmt.Sprintf("📋 *%s*\n\n`%s`", p.Name, p.Prompt))
			reply.ParseMode = "Markdown"
			b.api.Send(reply)
			break
		}
	}

	b.api.Request(tgbotapi.NewCallback(callback.ID, "已顯示 Prompt 內容"))
}

func (b *Bot) callbackHistory(callback *tgbotapi.CallbackQuery, idStr string) {
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	history, _ := b.db.GetHistory(callback.From.ID, 100)
	for _, h := range history {
		if h.ID == id {
			reply := tgbotapi.NewMessage(callback.Message.Chat.ID, fmt.Sprintf("📜 *歷史 Prompt*\n\n`%s`", h.Prompt))
			reply.ParseMode = "Markdown"
			b.api.Send(reply)
			break
		}
	}

	b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
}

func (b *Bot) callbackDefault(callback *tgbotapi.CallbackQuery, idStr string) {
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	if err := b.db.SetDefaultPrompt(callback.From.ID, id); err != nil {
		b.api.Request(tgbotapi.NewCallback(callback.ID, "設定失敗"))
		return
	}

	b.api.Request(tgbotapi.NewCallback(callback.ID, "✅ 已設定為預設"))

	// 重新顯示列表
	b.cmdSetDefault(callback.Message)
}

func (b *Bot) callbackQuality(callback *tgbotapi.CallbackQuery, quality string) {
	if err := b.db.SetUserSettings(callback.From.ID, quality); err != nil {
		b.api.Request(tgbotapi.NewCallback(callback.ID, "設定失敗"))
		return
	}

	b.api.Request(tgbotapi.NewCallback(callback.ID, fmt.Sprintf("✅ 預設畫質已設為 %s", quality)))

	// 更新訊息
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(qualityButton("1K", quality), "quality:1K"),
			tgbotapi.NewInlineKeyboardButtonData(qualityButton("2K", quality), "quality:2K"),
			tgbotapi.NewInlineKeyboardButtonData(qualityButton("4K", quality), "quality:4K"),
		),
	)

	edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID,
		fmt.Sprintf("⚙️ *設定*\n\n目前預設畫質：*%s*\n\n點擊更改：", quality))
	edit.ParseMode = "Markdown"
	edit.ReplyMarkup = &keyboard
	b.api.Send(edit)
}

func (b *Bot) callbackDelete(callback *tgbotapi.CallbackQuery, idStr string) {
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	if err := b.db.DeletePrompt(callback.From.ID, id); err != nil {
		b.api.Request(tgbotapi.NewCallback(callback.ID, "刪除失敗"))
		return
	}

	b.api.Request(tgbotapi.NewCallback(callback.ID, "✅ 已刪除"))

	// 重新顯示列表
	b.cmdDelete(callback.Message)
}

func (b *Bot) handlePhoto(msg *tgbotapi.Message) {
	// 解析參數
	caption := msg.Caption
	quality := ""
	withVoice := false
	customPrompt := ""

	// 檢查參數
	if strings.Contains(caption, "/s ") {
		// 解析畫質設定
		parts := strings.Split(caption, "/s ")
		if len(parts) > 1 {
			qParts := strings.Fields(parts[1])
			if len(qParts) > 0 {
				q := strings.ToUpper(qParts[0])
				if q == "1K" || q == "2K" || q == "4K" {
					quality = q
				}
			}
		}
		caption = strings.Split(caption, "/s")[0]
	}

	if strings.Contains(caption, "/v") {
		withVoice = true
		caption = strings.ReplaceAll(caption, "/v", "")
	}

	caption = strings.TrimSpace(caption)
	if caption != "" && !strings.HasPrefix(caption, "/") {
		customPrompt = caption
	}

	// 取得預設設定
	if quality == "" {
		quality, _ = b.db.GetUserSettings(msg.From.ID)
		if quality == "" {
			quality = "2K"
		}
	}

	// 決定使用的 Prompt
	prompt := config.DefaultPrompt
	if customPrompt != "" {
		prompt = customPrompt
		// 記錄到歷史
		b.db.AddToHistory(msg.From.ID, prompt)
	} else {
		// 檢查是否有使用者設定的預設
		defaultPrompt, _ := b.db.GetDefaultPrompt(msg.From.ID)
		if defaultPrompt != nil {
			prompt = defaultPrompt.Prompt
		}
	}

	// 發送處理中訊息
	processingMsg, _ := b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "⏳ 處理中..."))

	// 下載圖片
	photo := msg.Photo[len(msg.Photo)-1] // 取最大的圖片
	fileConfig := tgbotapi.FileConfig{FileID: photo.FileID}
	file, err := b.api.GetFile(fileConfig)
	if err != nil {
		b.updateMessage(processingMsg, "❌ 無法取得圖片")
		return
	}

	imageData, mimeType, err := b.downloadFile(file.FilePath)
	if err != nil {
		b.updateMessage(processingMsg, "❌ 下載圖片失敗")
		return
	}

	// 重試邏輯：2K 三次 → 1K 三次
	var result *gemini.ImageResult
	qualities := []string{quality, quality, quality, "1K", "1K", "1K"}
	if quality == "1K" {
		qualities = []string{"1K", "1K", "1K", "1K", "1K", "1K"}
	}

	ctx := context.Background()
	var lastErr error

	for i, q := range qualities {
		b.updateMessage(processingMsg, fmt.Sprintf("⏳ 處理中... (嘗試 %d/6，畫質 %s)", i+1, q))

		result, lastErr = b.gemini.GenerateImage(ctx, imageData, mimeType, prompt, q)
		if lastErr == nil {
			break
		}

		log.Printf("Attempt %d failed: %v", i+1, lastErr)
		time.Sleep(time.Second * 2)
	}

	if lastErr != nil {
		b.updateMessage(processingMsg, fmt.Sprintf("❌ 處理失敗（已重試 6 次）\n錯誤：%s", lastErr.Error()))
		return
	}

	// 如果需要語音
	var extractedText string
	var ttsResult *gemini.TTSResult

	if withVoice {
		b.updateMessage(processingMsg, "⏳ 擷取文字中...")
		extractedText, _ = b.gemini.ExtractText(ctx, imageData, mimeType, config.ExtractTextPrompt)

		if extractedText != "" {
			b.updateMessage(processingMsg, "⏳ 生成語音中...")
			ttsResult, _ = b.gemini.GenerateTTS(ctx, extractedText, config.TTSVoiceName)
		}
	}

	// 刪除處理中訊息
	b.api.Request(tgbotapi.NewDeleteMessage(msg.Chat.ID, processingMsg.MessageID))

	// 發送結果
	if withVoice && ttsResult != nil {
		// 使用 Media Group 同時發送圖片和音訊
		mediaGroup := tgbotapi.NewMediaGroup(msg.Chat.ID, []interface{}{
			tgbotapi.NewInputMediaPhoto(tgbotapi.FileBytes{Name: "translated.png", Bytes: result.ImageData}),
			tgbotapi.NewInputMediaAudio(tgbotapi.FileBytes{Name: "voice.wav", Bytes: ttsResult.AudioData}),
		})
		mediaGroup.ReplyToMessageID = msg.MessageID
		b.api.SendMediaGroup(mediaGroup)
	} else {
		// 只發送圖片
		photoMsg := tgbotapi.NewPhoto(msg.Chat.ID, tgbotapi.FileBytes{Name: "translated.png", Bytes: result.ImageData})
		photoMsg.ReplyToMessageID = msg.MessageID
		b.api.Send(photoMsg)
	}
}

func (b *Bot) downloadFile(filePath string) ([]byte, string, error) {
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.config.BotToken, filePath)
	resp, err := http.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	mimeType := "image/jpeg"
	if strings.HasSuffix(filePath, ".png") {
		mimeType = "image/png"
	}

	return data, mimeType, nil
}

func (b *Bot) updateMessage(msg tgbotapi.Message, text string) {
	edit := tgbotapi.NewEditMessageText(msg.Chat.ID, msg.MessageID, text)
	b.api.Send(edit)
}

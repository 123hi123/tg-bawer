package bot

import (
	"fmt"
	"log"
	"strings"

	"tg-bawer/database"
	"tg-bawer/gemini"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const failureAlertThreshold = 100

func normalizeGoogleModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return gemini.DefaultImageModel
	}
	return model
}

func normalizeModelName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "unknown"
	}
	return model
}

type modelStatTarget struct {
	OwnerUserID int64
	Provider    string
	Model       string
}

func buildGoogleStatTarget(ownerUserID int64, model string) modelStatTarget {
	return modelStatTarget{
		OwnerUserID: ownerUserID,
		Provider:    "google",
		Model:       normalizeGoogleModel(model),
	}
}

func buildGrokStatTarget(ownerUserID int64, model string) modelStatTarget {
	return modelStatTarget{
		OwnerUserID: ownerUserID,
		Provider:    "grok",
		Model:       normalizeModelName(model),
	}
}

func displayProviderName(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "google":
		return "Google"
	case "grok":
		return "Grok"
	case "unknown":
		return "Unknown"
	default:
		if provider == "" {
			return "Unknown"
		}
		return provider
	}
}

func (b *Bot) recordModelUsage(target modelStatTarget) {
	target.Provider = strings.ToLower(strings.TrimSpace(target.Provider))
	target.Model = normalizeModelName(target.Model)

	if err := b.db.RecordModelRequest(target.OwnerUserID, target.Provider, target.Model, 1, 0, 0); err != nil {
		log.Printf("記錄模型統計失敗(owner=%d, provider=%s, model=%s): %v", target.OwnerUserID, target.Provider, target.Model, err)
	}
}

func (b *Bot) recordModelSuccess(target modelStatTarget) {
	target.Provider = strings.ToLower(strings.TrimSpace(target.Provider))
	target.Model = normalizeModelName(target.Model)

	if err := b.db.RecordModelRequest(target.OwnerUserID, target.Provider, target.Model, 0, 1, 0); err != nil {
		log.Printf("記錄模型成功統計失敗(owner=%d, provider=%s, model=%s): %v", target.OwnerUserID, target.Provider, target.Model, err)
	}
}

func (b *Bot) recordModelFailure(target modelStatTarget) {
	target.Provider = strings.ToLower(strings.TrimSpace(target.Provider))
	target.Model = normalizeModelName(target.Model)

	if err := b.db.RecordModelRequest(target.OwnerUserID, target.Provider, target.Model, 0, 0, 1); err != nil {
		log.Printf("記錄模型失敗統計失敗(owner=%d, provider=%s, model=%s): %v", target.OwnerUserID, target.Provider, target.Model, err)
	}
}

func (b *Bot) cmdQueue(msg *tgbotapi.Message) {
	counts, err := b.db.GetFailedGenerationCounts(msg.From.ID)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ 讀取重試佇列失敗："+err.Error())
		reply.ReplyToMessageID = msg.MessageID
		b.api.Send(reply)
		return
	}

	googleCount := counts[taskTypeGoogleImage] + counts["google"]
	grokImgCount := counts[taskTypeGrokImage] + counts["grok"]
	grokVideoCount := counts[taskTypeGrokVideo]
	total := googleCount + grokImgCount + grokVideoCount

	var sb strings.Builder
	sb.WriteString("♻️ 你的待重試佇列\n\n")
	sb.WriteString(fmt.Sprintf("🌐 Google 圖片：%d\n", googleCount))
	sb.WriteString(fmt.Sprintf("🎭 Grok 圖片：%d\n", grokImgCount))
	sb.WriteString(fmt.Sprintf("🎬 Grok 影片：%d\n", grokVideoCount))
	sb.WriteString(fmt.Sprintf("📋 總計：%d", total))
	if total == 0 {
		sb.WriteString("\n\n✅ 目前沒有待重試任務")
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, sb.String())
	reply.ReplyToMessageID = msg.MessageID
	b.api.Send(reply)
}

func (b *Bot) cmdRecord(msg *tgbotapi.Message) {
	var (
		stats []database.ModelRequestStat
		err   error
		title string
	)
	if b.isAdminUser(msg.From.ID) {
		stats, err = b.db.GetGlobalModelRequestStats()
		title = "📊 全域模型請求統計"
	} else {
		stats, err = b.db.GetUserModelRequestStats(msg.From.ID)
		title = "📊 你的服務使用統計"
	}
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ 讀取模型統計失敗："+err.Error())
		reply.ReplyToMessageID = msg.MessageID
		b.api.Send(reply)
		return
	}

	if len(stats) == 0 {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "📊 目前沒有模型請求統計")
		reply.ReplyToMessageID = msg.MessageID
		b.api.Send(reply)
		return
	}

	var sb strings.Builder
	var alerts []string

	sb.WriteString(title + "\n")
	for i, stat := range stats {
		if i > 0 {
			sb.WriteString("\n")
		}
		title := fmt.Sprintf("%d. %s | %s", i+1, displayProviderName(stat.Provider), stat.ModelName)
		sb.WriteString("\n")
		sb.WriteString(title)
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("總請求：%d\n", stat.TotalCount))
		sb.WriteString(fmt.Sprintf("成功：%d\n", stat.SuccessCount))
		sb.WriteString(fmt.Sprintf("失敗：%d", stat.FailureCount))
		if stat.FailureCount > failureAlertThreshold {
			alerts = append(alerts, fmt.Sprintf("• %s | %s：%d 次", displayProviderName(stat.Provider), stat.ModelName, stat.FailureCount))
		}
	}

	if len(alerts) > 0 {
		sb.WriteString("\n\n⚠️ 失敗超過 100 次\n")
		sb.WriteString(strings.Join(alerts, "\n"))
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, sb.String())
	reply.ReplyToMessageID = msg.MessageID
	b.api.Send(reply)
}

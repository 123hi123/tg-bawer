package bot

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"tg-bawer/database"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) maintenanceText(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "資料庫檢查中"
	}
	return fmt.Sprintf("🛠 服務維修中\n\n%s\n此指令暫不受理，請稍後再試。", reason)
}

func (b *Bot) setMaintenanceMode(active bool, reason string) {
	b.maintenanceMu.Lock()
	defer b.maintenanceMu.Unlock()

	b.maintenanceActive = active
	if active {
		b.maintenanceReason = reason
	} else {
		b.maintenanceReason = ""
	}
}

func (b *Bot) maintenanceState() (bool, string) {
	b.maintenanceMu.RLock()
	defer b.maintenanceMu.RUnlock()
	return b.maintenanceActive, b.maintenanceReason
}

func (b *Bot) beginMaintenance(reason string) bool {
	b.maintenanceMu.Lock()
	defer b.maintenanceMu.Unlock()

	if b.maintenanceActive {
		return false
	}
	b.maintenanceActive = true
	b.maintenanceReason = reason
	return true
}

func (b *Bot) endMaintenance() {
	b.setMaintenanceMode(false, "")
}

func (b *Bot) rejectWhenMaintaining(msg *tgbotapi.Message) bool {
	active, reason := b.maintenanceState()
	if !active || b.isAdminUser(msg.From.ID) {
		return false
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.maintenanceText(reason))
	reply.ReplyToMessageID = msg.MessageID
	b.api.Send(reply)
	return true
}

func (b *Bot) rejectCallbackWhenMaintaining(callback *tgbotapi.CallbackQuery) bool {
	active, reason := b.maintenanceState()
	if !active || b.isAdminUser(callback.From.ID) {
		return false
	}
	b.api.Request(tgbotapi.NewCallback(callback.ID, b.maintenanceText(reason)))
	return true
}

func (b *Bot) waitForRetryWorkers(timeout time.Duration) int32 {
	deadline := time.Now().Add(timeout)
	for {
		active := atomic.LoadInt32(&b.activeRetries)
		if active == 0 || time.Now().After(deadline) {
			return active
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func buildFixDatabaseSummary(report *database.DatabaseFixReport, pendingRetries int32) string {
	var sb strings.Builder
	sb.WriteString("🛠 資料庫檢查完成\n\n")
	status := "OK"
	if !report.IntegrityOK {
		status = "FAILED"
	}
	sb.WriteString(fmt.Sprintf("SQLite integrity check：%s\n", status))
	if len(report.IntegrityMessages) > 0 {
		sb.WriteString("詳細結果：\n")
		for _, item := range report.IntegrityMessages {
			sb.WriteString("• " + item + "\n")
		}
	}
	sb.WriteString(fmt.Sprintf("修正預設服務：%d\n", report.UsersDefaultFixed))
	sb.WriteString(fmt.Sprintf("刪除無效服務：%d\n", report.InvalidServicesDeleted))
	if len(report.InvalidServiceIDs) > 0 {
		var ids []string
		for _, id := range report.InvalidServiceIDs {
			ids = append(ids, fmt.Sprintf("#%d", id))
		}
		sb.WriteString("刪除的服務 ID：" + strings.Join(ids, ", ") + "\n")
	}
	if pendingRetries > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️ 仍有 %d 個背景重試在收尾，新的重試已暫停到維修結束。", pendingRetries))
	}
	return strings.TrimSpace(sb.String())
}

func (b *Bot) cmdFix(msg *tgbotapi.Message) {
	if !b.isAdminUser(msg.From.ID) {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ 此指令僅限管理員使用")
		reply.ReplyToMessageID = msg.MessageID
		b.api.Send(reply)
		return
	}

	args := strings.Fields(msg.CommandArguments())
	if len(args) != 1 || strings.ToLower(args[0]) != "database" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ 用法：/fix database")
		reply.ReplyToMessageID = msg.MessageID
		b.api.Send(reply)
		return
	}

	if !b.beginMaintenance("資料庫檢查與修復中") {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "⚠️ 目前已有維修作業進行中")
		reply.ReplyToMessageID = msg.MessageID
		b.api.Send(reply)
		return
	}
	defer b.endMaintenance()

	statusMsg, err := b.sendReplyMessage(msg, "🛠 開始資料庫檢查...\n正在暫停背景重試與受理中的操作")
	if err == nil {
		b.updateMessage(statusMsg, "🛠 資料庫維修中...\n等待背景重試停止")
	}

	pendingRetries := b.waitForRetryWorkers(15 * time.Second)
	if err == nil {
		b.updateMessage(statusMsg, "🛠 資料庫維修中...\n執行 integrity check 與資料修復")
	}

	report, fixErr := b.db.FixDatabase()
	if fixErr != nil {
		text := "❌ 資料庫修復失敗：" + fixErr.Error()
		if err == nil {
			b.updateMessage(statusMsg, text)
		} else {
			reply := tgbotapi.NewMessage(msg.Chat.ID, text)
			reply.ReplyToMessageID = msg.MessageID
			b.api.Send(reply)
		}
		return
	}

	resultText := buildFixDatabaseSummary(report, pendingRetries)
	if err == nil {
		b.updateMessage(statusMsg, resultText)
	} else {
		reply := tgbotapi.NewMessage(msg.Chat.ID, resultText)
		reply.ReplyToMessageID = msg.MessageID
		b.api.Send(reply)
	}
}

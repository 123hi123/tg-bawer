package bot

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"tg-bawer/database"
	"tg-bawer/gemini"
	"tg-bawer/grok"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) cmdService(msg *tgbotapi.Message) {
	args := strings.Fields(msg.CommandArguments())
	if len(args) == 0 {
		b.sendServiceHelp(msg)
		return
	}

	switch strings.ToLower(args[0]) {
	case "help":
		b.sendServiceHelp(msg)
	case "list":
		b.sendServiceList(msg)
	case "add":
		// Reject /service add in group chats to protect API keys
		if !msg.Chat.IsPrivate() {
			b.api.Request(tgbotapi.NewDeleteMessage(msg.Chat.ID, msg.MessageID))
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "🔒 為了安全，請私聊 Bot 新增服務"))
			return
		}
		b.cmdServiceAdd(msg, args)
	case "use":
		b.cmdServiceUse(msg, args)
	case "delete", "del", "rm":
		b.cmdServiceDelete(msg, args)
	case "pub", "pri":
		// /service pub <id> or /service pri <id>
		b.cmdServicePublic(msg, args)
	default:
		// Check if first arg is a numeric ID: /service <id> pub/pri
		if _, err := strconv.ParseInt(args[0], 10, 64); err == nil && len(args) >= 2 {
			reordered := []string{strings.ToLower(args[1]), args[0]}
			b.cmdServicePublic(msg, reordered)
			return
		}
		b.sendServiceHelp(msg)
	}
}

func (b *Bot) sendServiceHelp(msg *tgbotapi.Message) {
	helpText := `🔌 *服務管理*

你可以新增四種服務來源：
1) ` + "`standard`" + `：只填 API Key（官方 Gemini）
2) ` + "`custom`" + `：自訂 Base URL + API Key
	3) ` + "`vertex`" + `：Vertex（支援只填 API Key 的 express mode）
4) ` + "`grok`" + `：Grok 影像生成（自訂 Base URL + API Key）

*指令格式：*
` + "`/service list`" + `
` + "`/service use <服務ID>`" + `
` + "`/service delete <服務ID>`" + `
` + "`/service <服務ID> pub`" + ` — 設為公開
` + "`/service <服務ID> pri`" + ` — 設為私人

` + "`/service add standard <名稱> <API_KEY>`" + `
` + "`/service add custom <名稱> <BASE_URL> <API_KEY>`" + `
	` + "`/service add vertex <名稱> <API_KEY>`" + `  (express mode)
	` + "`/service add vertex <名稱> <API_KEY> <PROJECT_ID> <LOCATION> [MODEL] [BASE_URL]`" + `  (full mode)
` + "`/service add grok <名稱> <BASE_URL> <API_KEY>`" + `
` + "`/service add grok custom <名稱> <BASE_URL> <API_KEY>`" + `

*範例：*
` + "`/service add standard my-gemini AIza...`" + `
` + "`/service add custom my-proxy https://your-proxy.example.com AIza...`" + `
	` + "`/service add vertex my-vertex AIza...`" + `
	` + "`/service add vertex my-vertex AIza... my-project asia-east1 gemini-3-pro-image-preview`" + `
` + "`/service add grok my-grok http://your-grok-host:8000 sk-...`" + `
` + "`/service add grok custom 66 http://48.218.144.171:53768 sk-...`"

	reply := tgbotapi.NewMessage(msg.Chat.ID, helpText)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

func (b *Bot) sendServiceList(msg *tgbotapi.Message) {
	isPrivate := msg.Chat.IsPrivate()

	services, err := b.db.GetUserServices(msg.From.ID)
	if err != nil {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 讀取服務列表失敗："+err.Error()))
		return
	}

	var lines []string

	if isPrivate {
		// Private chat: show full details
		lines = append(lines, "🔌 你的服務列表：")

		for _, service := range services {
			defaultMark := ""
			if service.IsDefault {
				defaultMark = " [預設]"
			}
			publicMark := " [私人]"
			if service.IsPublic {
				publicMark = " [公開]"
			}

			detail := fmt.Sprintf(
				"#%d %s (%s)%s%s key=%s",
				service.ID,
				service.Name,
				service.Type,
				defaultMark,
				publicMark,
				maskSecret(service.APIKey),
			)

			if (service.Type == gemini.ServiceTypeCustom || service.Type == grok.ServiceTypeGrok) && service.BaseURL != "" {
				detail += " base=" + service.BaseURL
			}

			if service.Type == gemini.ServiceTypeVertex {
				if service.ProjectID != "" && service.Location != "" {
					detail += fmt.Sprintf(" project=%s location=%s", service.ProjectID, service.Location)
				} else {
					detail += " mode=express"
				}
				if service.Model != "" {
					detail += " model=" + service.Model
				}
				if service.BaseURL != "" {
					detail += " base=" + service.BaseURL
				}
			}

			lines = append(lines, detail)
		}

		if len(services) == 0 {
			lines = append(lines, "（尚未新增服務）")
		}

		if strings.TrimSpace(b.config.GeminiAPIKey) != "" {
			lines = append(lines, "ENV fallback: GEMINI_API_KEY 已設定")
		}

		// Show available public shared services (name + type only, no sensitive info)
		publicServices, err := b.db.GetPublicServices()
		if err == nil {
			sharedLines := b.formatSharedServicesPrivate(publicServices, msg.From.ID)
			if len(sharedLines) > 0 {
				lines = append(lines, "")
				lines = append(lines, "🌐 可用的共用服務：")
				lines = append(lines, sharedLines...)
			}
		}
	} else {
		// Group chat: hide all sensitive info
		lines = append(lines, "🔌 你的服務列表（群組模式，隱藏敏感資訊）：")

		for _, service := range services {
			defaultMark := ""
			if service.IsDefault {
				defaultMark = " [預設]"
			}
			publicMark := " [私人]"
			if service.IsPublic {
				publicMark = " [公開]"
			}
			lines = append(lines, fmt.Sprintf("#%d %s (%s)%s%s", service.ID, service.Name, service.Type, defaultMark, publicMark))
		}

		if len(services) == 0 {
			lines = append(lines, "（尚未新增服務）")
		}

		// Show shared public services (no sensitive info)
		publicServices, err := b.db.GetPublicServices()
		if err == nil {
			sharedLines := b.formatSharedServicesGroup(publicServices, msg.From.ID)
			if len(sharedLines) > 0 {
				lines = append(lines, "")
				lines = append(lines, sharedLines...)
			}
		}

		lines = append(lines, "")
		lines = append(lines, "💡 私聊 Bot 可查看完整資訊")
	}

	lines = append(lines, "")
	lines = append(lines, "用 /service help 查看新增格式")

	b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, strings.Join(lines, "\n")))
}

func (b *Bot) cmdServiceAdd(msg *tgbotapi.Message, args []string) {
	if len(args) < 2 {
		b.sendServiceHelp(msg)
		return
	}

	mode := strings.ToLower(args[1])
	switch mode {
	case "standard", "gemini", "origin", "original":
		if len(args) < 4 {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 格式：/service add standard <名稱> <API_KEY>"))
			return
		}

		id, err := b.db.AddUserService(
			msg.From.ID,
			gemini.ServiceTypeStandard,
			args[2],
			args[3],
			"",
			"",
			"",
			"",
			true,
		)
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 新增 standard 服務失敗："+err.Error()))
			return
		}

		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ 已新增 standard 服務 #%d，並設為預設", id)))

	case "custom":
		if len(args) < 5 {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 格式：/service add custom <名稱> <BASE_URL> <API_KEY>"))
			return
		}

		id, err := b.db.AddUserService(
			msg.From.ID,
			gemini.ServiceTypeCustom,
			args[2],
			args[4],
			args[3],
			"",
			"",
			"",
			true,
		)
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 新增 custom 服務失敗："+err.Error()))
			return
		}

		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ 已新增 custom 服務 #%d，並設為預設", id)))

	case "vertex":
		// 支援兩種格式：
		// 1) express mode: /service add vertex <名稱> <API_KEY>
		// 2) full mode:    /service add vertex <名稱> <API_KEY> <PROJECT_ID> <LOCATION> [MODEL] [BASE_URL]
		if len(args) < 4 {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 格式：/service add vertex <名稱> <API_KEY> 或 /service add vertex <名稱> <API_KEY> <PROJECT_ID> <LOCATION> [MODEL] [BASE_URL]"))
			return
		}

		projectID := ""
		location := ""
		model := ""
		baseURL := ""

		if len(args) >= 6 {
			projectID = args[4]
			location = args[5]
		}
		if len(args) >= 7 {
			model = args[6]
		}
		if len(args) >= 8 {
			baseURL = args[7]
		}

		id, err := b.db.AddUserService(
			msg.From.ID,
			gemini.ServiceTypeVertex,
			args[2],
			args[3],
			baseURL,
			projectID,
			location,
			model,
			true,
		)
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 新增 vertex 服務失敗："+err.Error()))
			return
		}

		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ 已新增 vertex 服務 #%d，並設為預設", id)))

	case "grok":
		// Formats:
		// 1) /service add grok <名稱> <BASE_URL> <API_KEY>
		// 2) /service add grok custom <名稱> <BASE_URL> <API_KEY>
		var name, baseURL, apiKey string
		if len(args) >= 3 && strings.ToLower(args[2]) == "custom" {
			// explicit "custom" sub-type keyword
			if len(args) < 6 {
				b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 格式：/service add grok custom <名稱> <BASE_URL> <API_KEY>"))
				return
			}
			name = args[3]
			baseURL = args[4]
			apiKey = args[5]
		} else {
			if len(args) < 5 {
				b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 格式：/service add grok <名稱> <BASE_URL> <API_KEY>"))
				return
			}
			name = args[2]
			baseURL = args[3]
			apiKey = args[4]
		}

		id, err := b.db.AddUserService(
			msg.From.ID,
			grok.ServiceTypeGrok,
			name,
			apiKey,
			baseURL,
			"",
			"",
			"",
			true,
		)
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 新增 grok 服務失敗："+err.Error()))
			return
		}

		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ 已新增 grok 服務 #%d，並設為預設", id)))

	default:
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 不支援的服務類型，請用 standard/custom/vertex/grok"))
	}
}

func (b *Bot) cmdServiceUse(msg *tgbotapi.Message, args []string) {
	if len(args) < 2 {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 格式：/service use <服務ID>"))
		return
	}

	serviceID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 服務 ID 必須是數字"))
		return
	}

	if err := b.db.SetDefaultUserService(msg.From.ID, serviceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 找不到該服務 ID，請先用 /service list 查詢"))
			return
		}
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 設定預設服務失敗："+err.Error()))
		return
	}

	b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ 已切換預設服務為 #%d", serviceID)))
}

func (b *Bot) cmdServiceDelete(msg *tgbotapi.Message, args []string) {
	if len(args) < 2 {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 格式：/service delete <服務ID>"))
		return
	}

	serviceID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 服務 ID 必須是數字"))
		return
	}

	if err := b.db.DeleteUserService(msg.From.ID, serviceID); err != nil {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 刪除服務失敗："+err.Error()))
		return
	}

	b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ 已刪除服務 #%d", serviceID)))
}

func (b *Bot) cmdServicePublic(msg *tgbotapi.Message, args []string) {
	// args[0] should be "pub" or "pri", args[1] should be the service ID
	if len(args) < 2 {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 格式：/service <服務ID> pub 或 /service <服務ID> pri"))
		return
	}

	action := strings.ToLower(args[0])
	if action != "pub" && action != "pri" {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 請使用 pub（公開）或 pri（私人）"))
		return
	}

	serviceID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 服務 ID 必須是數字"))
		return
	}

	isPublic := action == "pub"
	if err := b.db.SetUserServicePublic(msg.From.ID, serviceID, isPublic); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 找不到該服務 ID，請先用 /service list 查詢"))
			return
		}
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ 修改服務失敗："+err.Error()))
		return
	}

	status := "公開"
	if !isPublic {
		status = "私人"
	}
	b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ 服務 #%d 已設為%s", serviceID, status)))
}

// resolveAllServiceConfigs returns all available Gemini/Google service configs for a user (for rotation).
// Grok services are excluded here; use resolveGrokClient for those.
// If the user has no services configured, public services from other users are used as a fallback.
func (b *Bot) resolveAllServiceConfigs(userID int64) ([]gemini.ServiceConfig, error) {
	services, err := b.db.GetAllUserServices(userID)
	if err != nil {
		return nil, err
	}

	var configs []gemini.ServiceConfig
	for _, service := range services {
		// Grok services use a different API and client; skip them here.
		// Use resolveGrokClient() instead for Grok-based generation.
		if service.Type == grok.ServiceTypeGrok {
			continue
		}
		configs = append(configs, gemini.ServiceConfig{
			Type:      service.Type,
			Name:      service.Name,
			APIKey:    service.APIKey,
			BaseURL:   service.BaseURL,
			ProjectID: service.ProjectID,
			Location:  service.Location,
			Model:     service.Model,
		})
	}

	// If user has no own services, fall back to public services from other users
	if len(configs) == 0 {
		publicServices, err := b.db.GetPublicServices()
		if err == nil {
			for _, service := range publicServices {
				if service.UserID == userID || service.Type == grok.ServiceTypeGrok {
					continue
				}
				configs = append(configs, gemini.ServiceConfig{
					Type:      service.Type,
					Name:      service.Name,
					APIKey:    service.APIKey,
					BaseURL:   service.BaseURL,
					ProjectID: service.ProjectID,
					Location:  service.Location,
					Model:     service.Model,
				})
			}
		}
	}

	if len(configs) == 0 && strings.TrimSpace(b.config.GeminiAPIKey) != "" {
		configs = append(configs, gemini.ServiceConfig{
			Type:    gemini.ServiceTypeStandard,
			Name:    "env-default",
			APIKey:  b.config.GeminiAPIKey,
			BaseURL: b.config.GeminiBaseURL,
			Model:   b.config.GeminiModel,
		})
	}

	return configs, nil
}

// resolveGrokClient returns a Grok client for the given user.
// It first checks the user's DB-configured Grok services (default first), then falls back to
// public Grok services from other users, then to the environment-variable-configured client.
// Returns nil if no Grok service is available.
func (b *Bot) resolveGrokClient(userID int64) *grok.Client {
	services, err := b.db.GetAllUserServices(userID)
	if err == nil {
		for _, s := range services {
			if s.Type == grok.ServiceTypeGrok {
				// Empty strings for imgModel/editModel/videoModel use the package defaults.
				return grok.NewClient(s.APIKey, s.BaseURL, "", "", "")
			}
		}
	}

	// Fall back to public Grok services from other users
	publicServices, err := b.db.GetPublicServices()
	if err == nil {
		for _, s := range publicServices {
			if s.UserID != userID && s.Type == grok.ServiceTypeGrok {
				return grok.NewClient(s.APIKey, s.BaseURL, "", "", "")
			}
		}
	}

	if b.grokClient.Available() {
		return b.grokClient
	}
	return nil
}

func (b *Bot) resolveServiceConfig(userID int64) (gemini.ServiceConfig, string, error) {
	services, err := b.resolveAllServiceConfigs(userID)
	if err != nil {
		return gemini.ServiceConfig{}, "", err
	}

	if len(services) > 0 {
		return services[0], services[0].Name, nil
	}

	return gemini.ServiceConfig{}, "", fmt.Errorf("尚未設定服務，請先使用 /service add")
}

func maskSecret(secret string) string {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return "(empty)"
	}
	if len(trimmed) <= 8 {
		return "****"
	}
	return trimmed[:4] + "..." + trimmed[len(trimmed)-4:]
}

// formatSharedServicesPrivate returns display lines for public services from other users,
// for use in private chat (shows name and type only, no sensitive info).
func (b *Bot) formatSharedServicesPrivate(publicServices []database.UserService, ownerID int64) []string {
	var lines []string
	for _, s := range publicServices {
		if s.UserID != ownerID {
			lines = append(lines, fmt.Sprintf("• %s (%s)", s.Name, s.Type))
		}
	}
	return lines
}

// formatSharedServicesGroup returns display lines for public services from other users,
// for use in group chat (shows name and type only, no sensitive info).
func (b *Bot) formatSharedServicesGroup(publicServices []database.UserService, ownerID int64) []string {
	var lines []string
	for _, s := range publicServices {
		if s.UserID != ownerID {
			lines = append(lines, fmt.Sprintf("🌐 %s (%s) — 由其他使用者共享", s.Name, s.Type))
		}
	}
	return lines
}

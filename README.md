# TG-Bawer 🍌✏️

Telegram Bot powered by Gemini - 用 AI 畫你想要的圖！

> **Bawer** = **Ba**nana + Dra**wer** 🎨

## ✨ 功能特色

- 🖼️ **AI 圖片生成** - 輸入文字描述，AI 幫你生成圖片
- 🔄 **圖片編輯** - 回覆圖片並描述你想要的修改
- 📸 **多圖支援** - 一次上傳多張圖片，Bot 會全部抓取處理
- 🎭 **貼圖支援** - 可以用貼圖當作圖片素材
- 📐 **自訂比例** - 支援 @1:1 @16:9 @9:16 等多種比例
- 🎨 **畫質選擇** - @1K @2K @4K 三種畫質
- 💾 **Prompt 管理** - 保存、列出、設定預設 Prompt
- 👥 **群組支援** - 在群組中以 . 開頭觸發
- 🔌 **多服務來源** - 支援 standard / custom URL / Vertex 三種服務
- 🎨 **畫質不降級重試** - fallback 重試維持使用者指定畫質
- 🔄 **失敗重試佇列** - 失敗組合入庫，系統每 15 分鐘隨機重試 1 筆
- 📦 **雙輸出** - 同時輸出預覽圖和原始檔案

---

## 🚀 前置準備

在開始之前，你需要準備：

### 1. Gemini API Key（可選）

1. 前往 [Google AI Studio](https://aistudio.google.com/app/apikey)
2. 點擊 Create API Key
3. 複製你的 API Key

> 若不想在環境變數放 API Key，也可讓 Bot 啟動後用 `/service add ...` 手動新增。

### 2. Telegram Bot Token

1. 在 Telegram 搜尋 @BotFather
2. 發送 /newbot 建立新 Bot
3. 依照指示設定 Bot 名稱
4. 複製你的 Bot Token

---

## 📦 快速部署

### 一行部署（推薦）

```bash
docker run -d \
  --name tg-bawer \
  --restart unless-stopped \
  -e GEMINI_API_KEY=你的_GEMINI_API_KEY \
  -e BOT_TOKEN=你的_BOT_TOKEN \
  -v ~/.tg-bawer:/app/data \
  ghcr.io/123hi123/tg-bawer:latest
```

### 使用 Docker Compose

1. 建立 .env 檔案：
   ```
   GEMINI_API_KEY=你的_GEMINI_API_KEY
   BOT_TOKEN=你的_BOT_TOKEN
   ```

2. 啟動：
   ```bash
   docker-compose up -d
   ```

---

## 📖 使用方式

### 基本用法

| 操作 | 說明 |
|------|------|
| 直接輸入文字 | AI 根據描述生成圖片 |
| 回覆圖片 + 輸入文字 | AI 根據圖片和描述進行編輯 |
| 回覆文字 + 傳圖片 | 同上，另一種操作方式 |
| 上傳多張圖 + 回覆其一 | AI 會抓取所有圖片一起處理 |

### 使用範例

#### 1️⃣ 純文字生成圖片

直接輸入文字描述，AI 會幫你生成圖片：

![純文字生成](https://cdn.jsdelivr.net/gh/321hi123/typoraimgbed/img/image-20251206151826189.png)

#### 2️⃣ 多圖處理

上傳多張圖片（確保是 Media Group），然後回覆其中任一張：

![上傳多圖](https://cdn.jsdelivr.net/gh/321hi123/typoraimgbed/img/image-20251206151923134.png)

Bot 會自動抓取整個 Group 的所有圖片一起處理：

![回覆多圖](https://cdn.jsdelivr.net/gh/321hi123/typoraimgbed/img/image-20251206152036722.png)

#### 3️⃣ 用圖片/貼圖回覆文字

先發送文字描述，然後用圖片或貼圖回覆（Telegram 貼圖也可以當作圖片素材）：

![圖片回覆文字](https://cdn.jsdelivr.net/gh/321hi123/typoraimgbed/img/image-20251206152215758.png)

生成結果：

![生成結果](https://cdn.jsdelivr.net/gh/321hi123/typoraimgbed/img/image-20251206152257926.png)

### 參數設定

在文字中使用 @ 符號設定參數（前後需有空格）：

```
翻譯這張漫畫 @16:9 @4K
```

群組回覆圖片時，如果只想使用被回覆的那一張（不要整個 media group），可加上：

```
翻譯這張 @s
```

**支援的比例：**
@1:1 @2:3 @3:2 @3:4 @4:3 @4:5 @5:4 @9:16 @16:9 @21:9

**支援的畫質：**
@1K @2K @4K

> 💡 不指定比例時：
> - 有傳入圖片：會自動套用「最接近原圖」的支援比例
> - 沒有傳入圖片：預設使用 `1:1`

### 群組使用

在群組中，文字訊息需以 . 開頭才會觸發：

```
.幫我畫一隻貓 @16:9
```

### Bot 指令

| 指令 | 說明 |
|------|------|
| /start | 顯示使用說明 |
| /help | 顯示幫助 |
| /save 名稱 prompt | 保存 Prompt |
| /list | 列出已保存的 Prompt |
| /history | 查看使用歷史 |
| /setdefault | 設定預設 Prompt |
| /settings | 設定預設畫質 |
| /delete | 刪除已保存的 Prompt |
| /service | 服務管理（新增/切換/刪除） |

### 服務管理指令（`/service`）

```bash
# 查看說明與列表
/service help
/service list

# 三種手動新增方式
/service add standard <名稱> <API_KEY>
/service add custom <名稱> <BASE_URL> <API_KEY>
/service add vertex <名稱> <API_KEY>
/service add vertex <名稱> <API_KEY> <PROJECT_ID> <LOCATION> [MODEL] [BASE_URL]

# 切換 / 刪除
/service use <服務ID>
/service delete <服務ID>

# 設為公開 / 私人
/service <服務ID> pub
/service <服務ID> pri
```

---

## ⚙️ 環境變數

| 變數 | 必填 | 說明 |
|------|------|------|
| GEMINI_API_KEY | ❌ | Google Gemini API Key（可改用 `/service add`） |
| GEMINI_BASE_URL | ❌ | Gemini API Base URL（自訂代理用） |
| BOT_TOKEN | ✅ | Telegram Bot Token |
| DATA_DIR | ❌ | 資料目錄（預設 /app/data） |

---

## 🛠️ 本地開發

```bash
# Clone 專案
git clone https://github.com/123hi123/tg-bawer.git
cd tg-bawer

# 安裝依賴
go mod tidy

# 設定環境變數
export GEMINI_API_KEY=your_key
export GEMINI_BASE_URL=https://generativelanguage.googleapis.com
export BOT_TOKEN=your_token

# 執行
go run .
```

---

## 📄 License

MIT

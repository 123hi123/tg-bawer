# TG-Bawer 🍌✏️# TG-Bawer 🍌✏️



Telegram Bot powered by Gemini - 用 AI 畫你想要的圖！Telegram Bot powered by Gemini - 用 AI 畫你想要的圖！



> **Bawer** = **Ba**nana + Dra**wer** 🎨> **Bawer** = **Ba**nana + Dra**wer** 🎨



## ✨ 功能特色## 功能



- 🖼️ **AI 圖片生成** - 輸入文字描述，AI 幫你生成圖片- 🎨 **圖片翻譯** - 自動將漫畫文字翻譯成中文

- 🔄 **圖片編輯** - 回覆圖片並描述你想要的修改- 🔊 **語音朗讀** - 擷取文字並生成 TTS 語音 (使用 `/v` 參數)

- 📸 **多圖支援** - 一次上傳多張圖片，Bot 會全部抓取處理- 📝 **Prompt 管理** - 保存、列出、設定預設 Prompt

- 🎭 **貼圖支援** - 可以用貼圖當作圖片素材- 📜 **使用歷史** - 查看過往使用的 Prompt

- 📐 **自訂比例** - 支援 `@1:1` `@16:9` `@9:16` 等多種比例- ⚙️ **畫質設定** - 支援 1K/2K/4K 畫質

- 🎨 **畫質選擇** - `@1K` `@2K` `@4K` 三種畫質- 🔄 **自動重試** - 失敗時自動降級重試 (2K×3 → 1K×3)

- 💾 **Prompt 管理** - 保存、列出、設定預設 Prompt

- 👥 **群組支援** - 在群組中以 `.` 開頭觸發## 快速開始

- 🔄 **智慧重試** - 失敗時自動降級重試（最多 6 次）

- 📦 **雙輸出** - 同時輸出預覽圖和原始檔案### 一行部署（Linux）



---```bash

docker run -d --name tg-bawer --restart unless-stopped -e GEMINI_API_KEY=你的API_KEY -e BOT_TOKEN=你的BOT_TOKEN -v ~/.tg-bawer:/app/data ghcr.io/123hi123/tg-bawer:latest

## 🚀 前置準備```



在開始之前，你需要準備：### 使用 GitHub Container Registry 鏡像



### 1. Gemini API Key```bash

1. 前往 [Google AI Studio](https://aistudio.google.com/app/apikey)docker run -d \

2. 點擊「Create API Key」  --name tg-bawer \

3. 複製你的 API Key  --restart unless-stopped \

  -e GEMINI_API_KEY=your_key \

### 2. Telegram Bot Token  -e BOT_TOKEN=your_token \

1. 在 Telegram 搜尋 [@BotFather](https://t.me/BotFather)  -v ~/.tg-bawer:/app/data \

2. 發送 `/newbot` 建立新 Bot  ghcr.io/123hi123/tg-bawer:latest

3. 依照指示設定 Bot 名稱```

4. 複製你的 Bot Token

### 使用 Docker Compose（推薦）

---

1. 複製環境變數範本：

## 📦 快速部署   ```bash

   cp .env.example .env

### 一行部署（推薦）   ```



```bash2. 編輯 `.env` 填入你的 API Key：

docker run -d \   ```

  --name tg-bawer \   GEMINI_API_KEY=your_gemini_api_key

  --restart unless-stopped \   BOT_TOKEN=your_telegram_bot_token

  -e GEMINI_API_KEY=你的_GEMINI_API_KEY \   ```

  -e BOT_TOKEN=你的_BOT_TOKEN \

  -v ~/.tg-bawer:/app/data \3. 啟動：

  ghcr.io/123hi123/tg-bawer:latest   ```bash

```   docker-compose up -d

   ```

### 使用 Docker Compose

### 使用 Docker

1. 建立 `.env` 檔案：

   ``````bash

   GEMINI_API_KEY=你的_GEMINI_API_KEYdocker build -t tg-bawer .

   BOT_TOKEN=你的_BOT_TOKENdocker run -d \

   ```  --name tg-bawer \

  -e GEMINI_API_KEY=your_key \

2. 啟動：  -e BOT_TOKEN=your_token \

   ```bash  -v $(pwd)/data:/app/data \

   docker-compose up -d  tg-bawer

   ``````



---### 本地執行



## 📖 使用方式```bash

# 安裝依賴

### 基本用法go mod tidy



| 操作 | 說明 |# 設定環境變數

|------|------|export GEMINI_API_KEY=your_key

| 直接輸入文字 | AI 根據描述生成圖片 |export BOT_TOKEN=your_token

| 回覆圖片 + 輸入文字 | AI 根據圖片和描述進行編輯 |export DATA_DIR=./data

| 回覆文字 + 傳圖片 | 同上，另一種操作方式 |

| 上傳多張圖 + 回覆其一 | AI 會抓取所有圖片一起處理 |# 執行

go run .

### 參數設定```



在文字中使用 `@` 符號設定參數（前後需有空格）：## 使用方式



```### 基本用法

翻譯這張漫畫 @16:9 @4K

```| 操作 | 說明 |

|------|------|

**支援的比例：**| 直接傳圖片 | 使用預設 Prompt 翻譯 |

`@1:1` `@2:3` `@3:2` `@3:4` `@4:3` `@4:5` `@5:4` `@9:16` `@16:9` `@21:9`| 圖片 + 文字 | 使用該文字作為 Prompt |



**支援的畫質：**### 圖片參數

`@1K` `@2K` `@4K`

在圖片說明中使用：

> 💡 不指定比例時，AI 會自動決定最適合的比例

| 參數 | 說明 | 範例 |

### 群組使用|------|------|------|

| `/s <畫質>` | 設定畫質 | `/s 4K` |

在群組中，文字訊息需以 `.` 開頭才會觸發：| `/v` | 同時生成語音（一筆訊息發送圖片+音訊） | `/v` |

```

.幫我畫一隻貓 @16:9組合使用：傳圖片並在說明輸入 `/s 4K /v`

```

### Bot 指令

### Bot 指令

| 指令 | 說明 |

| 指令 | 說明 ||------|------|

|------|------|| `/start` | 顯示歡迎訊息和使用說明 |

| `/start` | 顯示使用說明 || `/help` | 顯示幫助 |

| `/help` | 顯示幫助 || `/save <名稱> <prompt>` | 保存 Prompt |

| `/save <名稱> <prompt>` | 保存 Prompt || `/list` | 列出已保存的 Prompt（可點擊複製） |

| `/list` | 列出已保存的 Prompt || `/history` | 查看使用歷史 |

| `/history` | 查看使用歷史 || `/setdefault` | 設定預設 Prompt |

| `/setdefault` | 設定預設 Prompt || `/settings` | 設定預設畫質 |

| `/settings` | 設定預設畫質 || `/delete` | 刪除已保存的 Prompt |

| `/delete` | 刪除已保存的 Prompt |

## 專案結構

---

```

## ⚙️ 環境變數tg-bawer/

├── main.go              # 程式入口

| 變數 | 必填 | 說明 |├── bot/

|------|------|------|│   └── bot.go           # Telegram Bot 處理邏輯

| `GEMINI_API_KEY` | ✅ | Google Gemini API Key |├── config/

| `BOT_TOKEN` | ✅ | Telegram Bot Token |│   └── config.go        # 設定與常數

| `DATA_DIR` | ❌ | 資料目錄（預設 `/app/data`） |├── database/

│   └── database.go      # SQLite 資料庫操作

---├── gemini/

│   └── client.go        # Gemini API 客戶端

## 🛠️ 本地開發├── Dockerfile           # Docker 建置檔

├── docker-compose.yml   # Docker Compose 設定

```bash├── .env.example         # 環境變數範本

# 安裝 Go 1.22+└── README.md

# https://go.dev/dl/```



# Clone 專案## 資料庫結構

git clone https://github.com/123hi123/tg-bawer.git

cd tg-bawerSQLite 資料庫 (`data/bot.db`) 包含：



# 安裝依賴- `saved_prompts` - 使用者保存的 Prompt

go mod tidy- `prompt_history` - Prompt 使用歷史

- `user_settings` - 使用者設定（預設畫質等）

# 設定環境變數

export GEMINI_API_KEY=your_key## 環境變數

export BOT_TOKEN=your_token

| 變數 | 必填 | 說明 |

# 執行|------|------|------|

go run .| `GEMINI_API_KEY` | ✅ | Google Gemini API Key |

```| `BOT_TOKEN` | ✅ | Telegram Bot Token |

| `DATA_DIR` | ❌ | 資料目錄（預設 `./data`） |

---

## 技術細節

## 📁 專案結構

### 重試邏輯

```

tg-bawer/當圖片生成失敗時：

├── main.go              # 程式入口1. 前 3 次使用使用者設定的畫質（預設 2K）

├── bot/2. 後 3 次降級為 1K

│   └── bot.go           # Telegram Bot 處理邏輯3. 共最多重試 6 次

├── config/

│   └── config.go        # 設定與常數### API 呼叫流程

├── database/

│   └── database.go      # SQLite 資料庫操作**一般模式：**

├── gemini/1. `gemini-3-pro-image-preview` → 翻譯圖片

│   └── client.go        # Gemini API 客戶端

├── Dockerfile**語音模式 (`/v`)：**

├── docker-compose.yml1. `gemini-2.5-flash` → 擷取原文

└── README.md2. `gemini-3-pro-image-preview` → 翻譯圖片

```3. `gemini-2.5-flash-preview-tts` → 生成語音



---## License



## 📄 LicenseMIT


MIT

# 建置階段
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 安裝 CGO 依賴（SQLite 需要）
RUN apk add --no-cache gcc musl-dev

# 複製 go.mod 和 go.sum
COPY go.mod go.sum* ./
RUN go mod download

# 複製原始碼
COPY . .

# 建置
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-linkmode external -extldflags "-static"' -o gemini-manga-bot .

# 執行階段
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# 從建置階段複製執行檔
COPY --from=builder /app/gemini-manga-bot .

# 建立資料目錄
RUN mkdir -p /app/data

# 環境變數
ENV GEMINI_API_KEY=""
ENV BOT_TOKEN=""
ENV DATA_DIR="/app/data"

# 執行
CMD ["./gemini-manga-bot"]

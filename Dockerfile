# 建置階段
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 1️⃣ 先只複製依賴檔案（這一層可以被快取！）
COPY go.mod go.sum ./

# 2️⃣ 下載依賴（只要 go.mod/go.sum 沒變，這一層就會用快取）
RUN go mod download

# 3️⃣ 複製原始碼
COPY . .

# 4️⃣ 編譯（移除 -a flag，讓 Go 使用建置快取）
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags '-s -w' -o gemini-manga-bot .

# 執行階段
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/gemini-manga-bot .

RUN mkdir -p /app/data

ENV GEMINI_API_KEY=""
ENV BOT_TOKEN=""
ENV DATA_DIR="/app/data"

CMD ["./gemini-manga-bot"]

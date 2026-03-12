FROM golang:1.24.2-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o aliasb ./cmd/main.go

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/aliasb .
EXPOSE 8090
CMD ["./aliasb"]

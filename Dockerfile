FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY . .

# Generate default config
RUN go run ./cmd/minecraft-gateway

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app ./cmd/minecraft-gateway

FROM alpine:latest

WORKDIR /srv

COPY --from=builder /app .
COPY --from=builder /src/config.json .

EXPOSE 25565

ENTRYPOINT ["./app"]
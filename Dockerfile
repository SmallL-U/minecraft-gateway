FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY . .

# Generate default config
RUN go run main.go

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app main.go

FROM alpine:latest

WORKDIR /srv

COPY --from=builder /app .
COPY --from=builder /src/config.json .
COPY --from=builder /src/whitelist.txt .

EXPOSE 25565

ENTRYPOINT ["./app"]
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache make

WORKDIR /src
COPY . .

# Generate default config
RUN go run ./cmd/minecraft-gateway

# Build the application
RUN CGO_ENABLED=0 GOOS=linux make build

FROM alpine:latest

WORKDIR /srv

COPY --from=builder /src/bin/minecraft-gateway ./app
COPY --from=builder /src/config.yml .

EXPOSE 25565

ENTRYPOINT ["./app"]

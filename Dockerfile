FROM alpine:latest

WORKDIR /srv

COPY build/app .
COPY config.json .

EXPOSE 8080

ENTRYPOINT ["./app"]
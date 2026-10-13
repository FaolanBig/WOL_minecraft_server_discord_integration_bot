FROM golang:1.27-alpine AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY src ./src
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/minecraft-discord-bot ./src

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
COPY --from=build /out/minecraft-discord-bot /usr/local/bin/minecraft-discord-bot
USER app
ENTRYPOINT ["/usr/local/bin/minecraft-discord-bot"]

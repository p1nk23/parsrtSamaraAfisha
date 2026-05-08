FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go test ./... && go build -o /parser ./cmd/parser

FROM alpine:3.20
RUN apk add --no-cache chromium ca-certificates tzdata postgresql-client
WORKDIR /app
COPY --from=build /parser /app/parser
ENV HTTP_ADDR=:8081 \
    DATA_PATH=/app/data/events.json \
    USE_BROWSER=true \
    BROWSER_BIN=/usr/bin/chromium-browser \
    BROWSER_TIMEOUT=45s \
    PSQL_BIN=/usr/bin/psql
EXPOSE 8081
CMD ["/app/parser"]

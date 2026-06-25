FROM golang:1.25.5-alpine AS builder
RUN apk add --no-cache build-base
WORKDIR /build
COPY . .
RUN go mod download && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o iyoroynet-api

FROM alpine:latest
COPY --from=builder /build/iyoroynet-api /usr/local/bin/iyoroynet-api
RUN mkdir -p /app/logs
WORKDIR /app
EXPOSE 8080
ENV SERVER__SERVER_ADDRESS=":8080" \
    LOG__LOG_LEVEL="info" \
    LOG__LOG_FILE="logs/app.log"
CMD ["/usr/local/bin/iyoroynet-api"]

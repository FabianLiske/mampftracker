# syntax=docker/dockerfile:1.7
FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.24-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY --from=frontend /src/cmd/server/web/ ./cmd/server/web/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /mampftracker ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S mampftracker \
    && adduser -S -G mampftracker -u 10001 mampftracker \
    && mkdir -p /data \
    && chown mampftracker:mampftracker /data
COPY --from=backend /mampftracker /usr/local/bin/mampftracker
USER 10001:10001
ENV PORT=8080 \
    DATABASE_PATH=/data/mampftracker.db
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/mampftracker"]

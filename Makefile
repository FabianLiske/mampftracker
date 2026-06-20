.PHONY: build dev test image

build:
	cd frontend && npm run build
	go build -o bin/mampftracker ./cmd/server

dev:
	go run ./cmd/server

test:
	go test ./...
	cd frontend && npm run build

image:
	docker build -t mampftracker:local .

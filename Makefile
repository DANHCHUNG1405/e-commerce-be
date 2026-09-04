.PHONY: run build test tidy up down

run:
	go run ./cmd/api

build:
	go build ./cmd/api

test:
	go test ./...

tidy:
	go mod tidy

up:
	docker compose up --build

down:
	docker compose down


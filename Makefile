.PHONY: up down logs ps generate build

up:
	docker compose up --build

down:
	docker compose down -v

logs:
	docker compose logs -f

ps:
	docker compose ps

generate:
	cd users-service && go mod tidy && go generate ./...
	cd todos-service && go mod tidy && go generate ./...

build:
	docker compose build

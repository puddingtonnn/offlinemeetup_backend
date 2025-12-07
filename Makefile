APP_NAME=meetuper
CMD=cmd/app/main.go

run:
	go run $(CMD)

build:
	go build -o bin/$(APP_NAME) $(CMD)

deps:
	go mod tidy

swag:
	go run github.com/swaggo/swag/cmd/swag@latest init -g $(CMD) -d ./
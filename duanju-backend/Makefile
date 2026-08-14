# 后端 Makefile（对齐 04/05 工程规范：lint/test/build/run/migrate/gen）
GOPROXY ?= https://goproxy.cn,direct
export GOPROXY
export GOSUMDB := off

.PHONY: all build run lint test tidy gen migrate-up migrate-down migrate-version migrate-create health

all: lint test build

build:
	go build -o bin/server ./cmd/api
	go build -o bin/worker ./cmd/worker
	go build -o bin/migrate ./cmd/migrate

run:
	go run ./cmd/api

lint:
	go vet ./...

test:
	go test ./... -count=1

tidy:
	go mod tidy

gen:
	go run ./gen/enumgen

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down 1

migrate-version:
	go run ./cmd/migrate version

migrate-create:
	@test -n "$(name)" || (echo "用法: make migrate-create name=add_xxx"; exit 1)
	go run ./cmd/migrate create -name $(name)

health:
	@powershell -Command "try { (Invoke-RestMethod http://127.0.0.1:8000/healthz).code } catch { Write-Host '服务未启动' }"

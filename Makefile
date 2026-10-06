.DEFAULT_GOAL := help
GOLANGCI_VERSION ?= v2.13.1

help: ## แสดงคำสั่งทั้งหมด
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "};{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## build binary
	go build -trimpath -o bin/chat-service ./cmd/server

vet: ## go vet
	go vet ./...

lint: ## golangci-lint (ใช้ binary ในเครื่องถ้ามี ไม่มีก็ใช้ docker)
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "ไม่พบ golangci-lint ในเครื่อง — ใช้ docker $(GOLANGCI_VERSION) แทน"; \
		docker run --rm -v "$(PWD)":/app -w /app \
			golangci/golangci-lint:$(GOLANGCI_VERSION) golangci-lint run ./...; \
	fi

test: ## unit test (ไม่ต้องใช้ docker)
	go test -race -count=1 ./...

tidy: ## go mod tidy + ตรวจว่าไม่มี diff
	go mod tidy
	@git diff --exit-code go.mod go.sum || { echo "go.mod/go.sum ไม่ tidy — commit การเปลี่ยนแปลงด้วย"; exit 1; }

docker-build: ## build image
	docker build -t chat-service:local .

.PHONY: help build vet lint test tidy docker-build

APP_NAME   := sms_service
IMAGE_NAME := sms_service
CONTAINER  := sms_service
PORT       := 5051

.PHONY: run build build-linux tidy lint \
        docker-build docker-run docker-stop docker-restart docker-logs

# ─── Development ──────────────────────────────────────────────────────────────

## run: Run the service locally (requires Redis on host)
run:
	@go run main.go

## tidy: Tidy go.mod / go.sum
tidy:
	@go mod tidy

# ─── Build ────────────────────────────────────────────────────────────────────

## build: Compile binary for the current OS
build:
	@go build -ldflags="-w -s" -o $(APP_NAME) .
	@echo "Built: ./$(APP_NAME)"

## build-linux: Cross-compile a static binary for Linux amd64 (Ubuntu)
build-linux:
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -ldflags="-w -s" -o $(APP_NAME) .
	@echo "Built Linux binary: ./$(APP_NAME)"

# ─── Lint ─────────────────────────────────────────────────────────────────────

## lint: Run golangci-lint (install: https://golangci-lint.run/usage/install/)
lint:
	@golangci-lint run ./...

## fmt: Format all Go source files
fmt:
	@gofmt -w .

# ─── Docker ───────────────────────────────────────────────────────────────────

## docker-build: Build the Docker image
docker-build:
	@docker build -t $(IMAGE_NAME) .
	@echo "Image built: $(IMAGE_NAME)"

## docker-run: Start the container (uses host network so it can reach host Redis)
docker-run:
	@docker run -d \
		--name $(CONTAINER) \
		--network host \
		--env-file .env \
		--restart unless-stopped \
		$(IMAGE_NAME)
	@echo "Container '$(CONTAINER)' started on port $(PORT)"

## docker-stop: Stop and remove the container
docker-stop:
	@docker stop $(CONTAINER) && docker rm $(CONTAINER)
	@echo "Container '$(CONTAINER)' stopped and removed"

## docker-restart: Restart the running container
docker-restart:
	@docker restart $(CONTAINER)
	@echo "Container '$(CONTAINER)' restarted"

## docker-logs: Tail container logs
docker-logs:
	@docker logs -f $(CONTAINER)

# ─── Help ─────────────────────────────────────────────────────────────────────

## help: List all available targets
help:
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## /  /'
	@echo ""

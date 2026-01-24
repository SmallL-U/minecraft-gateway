.PHONY: help build clean run reload stop

APP_NAME := minecraft-gateway
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP_NAME)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the application
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/minecraft-gateway

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)

run: build ## Build and run the application
	$(BIN)

reload: ## Reload configuration of running instance
	$(BIN) reload

stop: ## Stop running instance
	$(BIN) stop

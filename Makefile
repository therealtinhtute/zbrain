.DEFAULT_GOAL := help

.PHONY: help build test smoke install-local clean

help: ## Show available targets
	@printf "Available targets:\n"
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Compile the Go CLI
	go build -o dist/zbrain ./cmd/zbrain

test: ## Run the Go test suite
	go test ./...

smoke: build ## Run smoke checks against dist/zbrain
	./dist/zbrain --help
	@tmp_home=$$(mktemp -d /tmp/zbrain-smoke.XXXXXX); \
	ZBRAIN_HOME="$$tmp_home" ./dist/zbrain setup; \
	ZBRAIN_HOME="$$tmp_home" ./dist/zbrain workspace create research; \
	ZBRAIN_HOME="$$tmp_home" ./dist/zbrain workspace current; \
	trash "$$tmp_home"

install-local: build ## Install zbrain into ~/.local/bin
	mkdir -p "$$HOME/.local/bin"
	cp ./dist/zbrain "$$HOME/.local/bin/zbrain"
	chmod +x "$$HOME/.local/bin/zbrain"

clean: ## Remove generated build output
	trash dist

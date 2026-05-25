.DEFAULT_GOAL := help

.PHONY: help install build test typecheck smoke clean purge

help: ## Show available targets
	@printf "Available targets:\n"
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_-]+:.*## / {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install Bun dependencies and verify qmd is installed
	bun install
	@command -v qmd >/dev/null 2>&1 || { \
		printf "qmd is required but was not found on PATH.\n"; \
		printf "Run: npm i -g @tobilu/qmd\n"; \
		exit 1; \
	}

build: ## Compile the standalone CLI with Bun
	bun run build

test: ## Run the Bun test suite
	bun test --run

typecheck: ## Run the TypeScript type checker
	bunx tsc --noEmit

smoke: ## Run smoke checks against dist/zbrain
	@test -x ./dist/zbrain || { \
		printf "dist/zbrain is missing. Run 'make build' first.\n"; \
		exit 1; \
	}
	./dist/zbrain --help
	@tmp_home=$$(mktemp -d /tmp/zbrain-smoke.XXXXXX); \
	ZBRAIN_HOME="$$tmp_home" ./dist/zbrain setup; \
	rm -rf "$$tmp_home"

clean: ## Remove generated build output
	rm -rf dist

purge: ## Remove legacy/current runtimes and local project integration for a fresh start
	rm -rf dist
	rm -rf "$$HOME/.zwiki" "$$HOME/.zbrain"
	rm -rf .claude/commands .claude/agents
	rm -f .claude/zwiki.json .claude/zbrain.json .claude/settings.local.json

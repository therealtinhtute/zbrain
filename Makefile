.DEFAULT_GOAL := help

.PHONY: help build test smoke install-local clean bench eval eval-suite

help: ## Show available targets
	@printf "Available targets:\n"
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Compile the Rust CLI release binary (dist/zbrain + stripped)
	@mkdir -p dist
	cargo build --release -p zbrain --bin zbrain
	cp target/release/zbrain dist/zbrain
	strip dist/zbrain -o dist/zbrain.stripped 2>/dev/null || cp dist/zbrain dist/zbrain.stripped
	@ls -lh dist/zbrain* 2>/dev/null || true

test: ## Run the Rust test suite
	cargo test --workspace

smoke: build ## Run smoke checks against dist/zbrain in isolated ZBRAIN_HOME
	./scripts/smoke.sh --bin ./dist/zbrain

bench: ## Run ask p95 bench (100k corpus; set ZBRAIN_BENCH_100K=1, slow box)
	ZBRAIN_BENCH_100K=1 cargo test -p zbrain --test bench_100k -- --nocapture

eval: ## Run the retrieval eval suite
	cargo test -p zbrain --test eval_suite

eval-suite: eval ## Alias for eval

install-local: build ## Install zbrain into ~/.local/bin
	mkdir -p "$$HOME/.local/bin"
	cp ./dist/zbrain "$$HOME/.local/bin/zbrain"
	chmod +x "$$HOME/.local/bin/zbrain"

clean: ## Remove generated build output
	trash dist target 2>/dev/null || rm -rf dist target

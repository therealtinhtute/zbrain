.DEFAULT_GOAL := help

.PHONY: help build build-stripped test smoke install-local clean bench eval

help: ## Show available targets
	@printf "Available targets:\n"
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Compile the Go CLI (debug + stripped)
	@mkdir -p dist
	go build -o dist/zbrain ./cmd/zbrain
	go build -ldflags="-s -w" -trimpath -o dist/zbrain.stripped ./cmd/zbrain
	@ls -lh dist/zbrain* 2>/dev/null || true

build-stripped: ## Compile stripped binary (~14-16M, no debug_info)
	@mkdir -p dist
	go build -ldflags="-s -w" -trimpath -o dist/zbrain.stripped ./cmd/zbrain
	@ls -lh dist/zbrain.stripped
	@file dist/zbrain.stripped

bench: build ## Run FTS5/perf baseline (100, 1k)
	go run ./scripts/bench-fts5.go --sizes=100,1000

eval: build ## Run retrieval eval (P@K/R@K/MRR/NDCG) on 1k syn corpus
	go run ./internal/eval --corpus=1000 --limit=10 --json docs/proofs/eval-baseline.json
	@cat docs/proofs/eval-baseline.json | python3 -m json.tool | head -40

test: ## Run the Go test suite
	go test ./...

smoke: build ## Run smoke checks against dist/zbrain
	./dist/zbrain --help
	@tmp_root=$$(cd "$${TMPDIR:-/tmp}" && pwd -P); \
	tmp_home=$$(mktemp -d "$$tmp_root/zbrain-smoke.XXXXXX"); \
	source_file=$$(mktemp "$$tmp_root/zbrain-source.XXXXXX"); \
	printf 'trusted source bytes\n' > "$$source_file"; \
	ZBRAIN_HOME="$$tmp_home" ./dist/zbrain setup; \
	ZBRAIN_HOME="$$tmp_home" ./dist/zbrain workspace create research; \
	ZBRAIN_HOME="$$tmp_home" ./dist/zbrain workspace current; \
	evidence_json=$$(ZBRAIN_HOME="$$tmp_home" ./dist/zbrain evidence add --file "$$source_file" --origin "file://smoke" --media-type text/plain); \
	evidence_id=$$(printf '%s' "$$evidence_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'); \
	claim_json=$$(printf 'trusted smoke answer\n' | ZBRAIN_HOME="$$tmp_home" ./dist/zbrain claim draft --tier projects --title 'Smoke Claim' --basis evidence --evidence "$$evidence_id"); \
	claim_id=$$(printf '%s' "$$claim_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'); \
	ZBRAIN_HOME="$$tmp_home" ./dist/zbrain claim approve "$$claim_id"; \
	ZBRAIN_HOME="$$tmp_home" ./dist/zbrain reindex; \
	ZBRAIN_HOME="$$tmp_home" ./dist/zbrain ask trusted smoke; \
	if command -v trash >/dev/null 2>&1; then \
		trash "$$source_file"; \
		trash "$$tmp_home"; \
	else \
		rm -f "$$source_file"; \
		rm -rf "$$tmp_home"; \
	fi

install-local: build ## Install zbrain into ~/.local/bin
	mkdir -p "$$HOME/.local/bin"
	cp ./dist/zbrain "$$HOME/.local/bin/zbrain"
	chmod +x "$$HOME/.local/bin/zbrain"

clean: ## Remove generated build output
	trash dist

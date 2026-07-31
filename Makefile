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
	source_file=$$(mktemp /tmp/zbrain-source.XXXXXX); \
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
	trash "$$source_file"; \
	trash "$$tmp_home"

install-local: build ## Install zbrain into ~/.local/bin
	mkdir -p "$$HOME/.local/bin"
	cp ./dist/zbrain "$$HOME/.local/bin/zbrain"
	chmod +x "$$HOME/.local/bin/zbrain"

clean: ## Remove generated build output
	trash dist

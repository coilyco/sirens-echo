.PHONY: build vet test tidy format run run-echo eval-echo

OPS_BIN := bin/sirens-discord-ops
ECHO_BIN := bin/sirens-echo
ECHO_EVAL_BIN := bin/sirens-echo-eval

build: ## Compile the admin bot, Sirens Echo, and the Echo evaluation gate.
	@mkdir -p bin
	go build -o $(OPS_BIN) ./cmd/sirens-discord-ops
	go build -o $(ECHO_BIN) ./cmd/sirens-echo
	go build -o $(ECHO_EVAL_BIN) ./cmd/sirens-echo-eval

vet: ## go vet across the tree.
	go vet ./...

test: ## Run the unit test suite.
	go test ./...

tidy: ## go mod tidy.
	go mod tidy

format: ## Format Go source.
	gofmt -w ./cmd ./internal

# Local dev run against Sirens Echo. Export DISCORD_TOKEN, ADMIN_CHANNEL_ID,
# AUDIT_CHANNEL_ID, ADMIN_ROLE_ID first (or wrap in a direnv .envrc).
# Production deploy is `git push` here, then `git pull && bash scripts/install.sh`
# on kai-server.
run: ## Local dev run against Sirens Echo. Requires DISCORD_TOKEN + channel/role env vars.
	go run ./cmd/sirens-discord-ops

run-echo: ## Run the Sirens Echo community agent with deployment env.
	go run ./cmd/sirens-echo

eval-echo: ## Run the two non-mutating Sirens Echo cases through Agent Proxy.
	go run ./cmd/sirens-echo-eval

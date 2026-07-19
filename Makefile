# Tera Agent — build & test helpers. Owner: DevOps Engineer.
BIN      := tera-agent
CMD      := ./cmd/tera-agent
DIST     := dist

.PHONY: build test race vet cover run printers clean cross

build: ## Compile the agent for the host platform
	go build -o $(BIN) $(CMD)

test: ## Run unit tests
	go test ./...

race: ## Run tests with the race detector
	go test -race ./...

vet: ## Static checks
	go vet ./...

cover: ## Tests with coverage summary
	go test -cover ./...

run: build ## Run the agent (needs config.json)
	./$(BIN) run -config config.json

printers: build ## List printers (silent, via CUPS)
	./$(BIN) printers

clean:
	rm -f $(BIN)
	rm -rf $(DIST)

# Cross-compile release binaries (silent printing is implemented for linux/darwin;
# windows builds use the platform stub until the Windows Print API adapter lands).
cross:
	mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64 go build -o $(DIST)/$(BIN)-linux-amd64      $(CMD)
	GOOS=linux   GOARCH=arm64 go build -o $(DIST)/$(BIN)-linux-arm64      $(CMD)
	GOOS=darwin  GOARCH=amd64 go build -o $(DIST)/$(BIN)-darwin-amd64     $(CMD)
	GOOS=darwin  GOARCH=arm64 go build -o $(DIST)/$(BIN)-darwin-arm64     $(CMD)
	GOOS=windows GOARCH=amd64 go build -o $(DIST)/$(BIN)-windows-amd64.exe $(CMD)

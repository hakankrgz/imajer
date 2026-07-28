GO ?= go
VERSION ?= dev
DIST := dist
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: test vet build cross reproducible demo clean

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/imajer ./cmd/imajer
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/imajer-agent ./cmd/imajer-agent

cross:
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/imajer-darwin-amd64 ./cmd/imajer
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/imajer-darwin-arm64 ./cmd/imajer
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/imajer-windows-amd64.exe ./cmd/imajer
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/imajer-windows-arm64.exe ./cmd/imajer
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/imajer-agent-linux-amd64 ./cmd/imajer-agent
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/imajer-agent-linux-arm64 ./cmd/imajer-agent
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/imajer-agent-windows-amd64.exe ./cmd/imajer-agent

reproducible:
	@imajer_tmp=$$(mktemp -d); \
	trap 'rm -rf "$$imajer_tmp"' EXIT; \
	CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -ldflags "$(LDFLAGS)" -o "$$imajer_tmp/imajer-a" ./cmd/imajer; \
	CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -ldflags "$(LDFLAGS)" -o "$$imajer_tmp/imajer-b" ./cmd/imajer; \
	cmp "$$imajer_tmp/imajer-a" "$$imajer_tmp/imajer-b"; \
	CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -ldflags "$(LDFLAGS)" -o "$$imajer_tmp/agent-a" ./cmd/imajer-agent; \
	CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -ldflags "$(LDFLAGS)" -o "$$imajer_tmp/agent-b" ./cmd/imajer-agent; \
	cmp "$$imajer_tmp/agent-a" "$$imajer_tmp/agent-b"

demo: build
	./demo/prepare.sh

clean:
	rm -rf $(DIST)

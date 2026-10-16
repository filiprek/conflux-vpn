GO ?= go
.PHONY: build test race vet frontend check security
build:
	GO="$(GO)" bash scripts/build.sh
test:
	$(GO) test ./...
race:
	$(GO) test -race ./...
vet:
	$(GO) vet ./...
frontend:
	cd ui/desktop/frontend && npm ci && npm run typecheck && npm run lint && npm test && npm run build
check:
	GO="$(GO)" bash scripts/check.sh
security:
	mkdir -p .tools
	GOBIN="$(CURDIR)/.tools" $(GO) install golang.org/x/vuln/cmd/govulncheck@v1.9.0
	.tools/govulncheck ./...
	GOOS=windows .tools/govulncheck ./...
	cd ui/desktop && GOOS=windows ../../.tools/govulncheck -tags production ./...
	cd ui/desktop/frontend && npm audit --audit-level=high

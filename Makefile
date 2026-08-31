# TempestKeep build and verification commands.
# Go downloads tool modules through the checksum database.

GO ?= go
BIN_DIR ?= bin

GO_VERSION := 1.27.0
GOIMPORTS_VERSION := v0.44.0
GOLANGCI_LINT_VERSION := v2.12.2
ACTIONLINT_VERSION := v1.7.12
GOVULNCHECK_VERSION := v1.7.0
GITLEAKS_VERSION := v8.30.1
GO_LICENSES_VERSION := v1.6.0
CYCLONEDX_GOMOD_VERSION := v1.12.0
GORELEASER_VERSION := v2.18.0

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X github.com/lennrt/tempestkeep/internal/version.version=$(VERSION)"

.PHONY: all check-go download tidy tidy-check update-deps build build-pure build-arm64 vet fmt fmtcheck docs-check lint workflows vuln test integration e2e live-smoke race fuzz bench cover api-check api-update generated licenses sbom secrets verify mcp tempest demoapi agentdemo demo demo-setup demo-agent demo-explore vhs release-check hooks clean

all: build

check-go:
	@test "$$($(GO) env GOVERSION)" = "go$(GO_VERSION)" || { echo "Go $(GO_VERSION) is required."; exit 1; }

download: check-go
	$(GO) mod download
	$(GO) mod verify

tidy:
	$(GO) mod tidy

tidy-check:
	$(GO) mod tidy -diff

update-deps:
	$(GO) get -u ./...
	$(GO) mod tidy
	$(MAKE) test

build: mcp tempest

build-pure:
	CGO_ENABLED=0 $(GO) build ./...

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
	$(GO) run golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION) -w $$(find . -name '*.go' -not -path './vendor/*')

fmtcheck:
	@test -z "$$(gofmt -l .)" || { echo "gofmt is required for:"; gofmt -l .; exit 1; }
	@test -z "$$($(GO) run golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION) -l .)" || { echo "goimports is required."; exit 1; }

docs-check:
	$(GO) run ./tools/docscheck -root .

test:
	CGO_ENABLED=0 $(GO) test ./... -count=1 -timeout=5m

integration:
	CGO_ENABLED=0 $(GO) test ./pkg/tempest/api ./pkg/tempest/collect ./pkg/tempest/store -count=1 -timeout=2m

e2e:
	CGO_ENABLED=0 $(GO) test ./cmd/tempest-mcp -run '^(TestE2E|TestIntegration)' -count=1 -timeout=2m

live-smoke:
	./scripts/live-smoke.sh

race:
	CGO_ENABLED=1 $(GO) test -race ./... -count=1 -timeout=10m

fuzz:
	CGO_ENABLED=0 $(GO) test ./pkg/tempest/config -run '^$$' -fuzz '^FuzzDotenvValueRoundTrip$$' -fuzztime=5s -parallel=1
	CGO_ENABLED=0 $(GO) test ./pkg/tempest/model -run '^$$' -fuzz '^FuzzDeviceObsFromRow$$' -fuzztime=5s -parallel=1

bench:
	$(GO) test -bench=. -benchtime=3x -run='^$$' ./pkg/tempest/store

cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

workflows:
	$(GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

api-check:
	./scripts/check-public-api.sh

api-update:
	./scripts/check-public-api.sh --update

generated: api-check

licenses:
	@test "$$($(GO) list -m -f '{{.Version}}' modernc.org/mathutil)" = "v1.7.1"
	@test "$$(shasum -a 256 "$$($(GO) env GOMODCACHE)/modernc.org/mathutil@v1.7.1/LICENSE" | cut -d' ' -f1)" = "bfa9bf72a72ca009fd62a8f84fca3dca67e51d93af96352723646599898b6cf5"
	@test "$$($(GO) list -m -f '{{.Version}}' github.com/segmentio/asm)" = "v1.2.1"
	@test "$$(shasum -a 256 "$$($(GO) env GOMODCACHE)/github.com/segmentio/asm@v1.2.1/LICENSE" | cut -d' ' -f1)" = "cca993712df289a5958bdef69031a5dac0f951ac15afeb313f9eeea55ed59443"
	$(GO) run github.com/google/go-licenses@$(GO_LICENSES_VERSION) check ./... --ignore=modernc.org/mathutil --ignore=github.com/segmentio/asm --allowed_licenses=Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,MIT-0,MPL-2.0,Unicode-3.0

sbom:
	$(GO) run github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_GOMOD_VERSION) mod -json -output /tmp/tempestkeep-sbom.json

secrets:
	$(GO) run github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION) git --redact --no-banner --no-color --timeout=300 --log-opts=--all
	$(GO) run github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION) dir --redact --no-banner --no-color --timeout=300 --max-target-megabytes=20 .

verify: check-go download fmtcheck docs-check tidy-check vet test race fuzz lint workflows generated vuln licenses sbom secrets build-pure build-arm64

mcp:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/tempest-mcp ./cmd/tempest-mcp

tempest:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/tempest ./cmd/tempest

demoapi:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -o $(BIN_DIR)/demoapi ./internal/demo/mockapi

agentdemo:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -o $(BIN_DIR)/agentdemo ./internal/demo/agentdemo

demo: mcp tempest demoapi
	vhs docs/demo.tape

demo-setup: tempest demoapi
	vhs docs/setup.tape

demo-agent: mcp demoapi agentdemo
	vhs docs/agent.tape

demo-explore: tempest demoapi
	vhs docs/explore.tape

vhs: demo demo-setup demo-agent demo-explore

release-check:
	$(GO) run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION) release --snapshot --clean

hooks:
	git config core.hooksPath .githooks
	@echo "Installed the check-only hooks from .githooks."

clean:
	rm -f "$(BIN_DIR)/tempest" "$(BIN_DIR)/tempest-mcp" "$(BIN_DIR)/demoapi" "$(BIN_DIR)/agentdemo" coverage.out
	@rmdir "$(BIN_DIR)" 2>/dev/null || true
	rm -rf -- "$(CURDIR)/dist"

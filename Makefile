VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/ahmettsezis/flui/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)
IMAGE   ?= ghcr.io/ahmettsezis/flui

GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION   := v1.9.0
MARKDOWNLINT_VERSION  := 0.18.1

DEV_ENV := FLUI_DATA_DIR=$(CURDIR)/.dev-data FLUI_SERVER_LISTEN=127.0.0.1:8443 FLUI_LOG_FORMAT=text FLUI_LOG_LEVEL=debug

.PHONY: all build web test lint dev docker clean

all: lint test build

web/node_modules: web/package-lock.json
	npm --prefix web ci --no-fund --no-audit
	@touch $@

web: web/node_modules
	npm --prefix web run build
	@touch internal/webui/dist/.gitkeep

build: web
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/flui-server ./cmd/flui-server

test:
	go test -race -count=1 ./...

lint:
	go vet ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...
	npx --yes markdownlint-cli2@$(MARKDOWNLINT_VERSION) "**/*.md"

dev: web/node_modules
	@echo "UI: http://localhost:5173  (API: https://127.0.0.1:8443)"
	@trap 'kill 0' INT TERM; $(DEV_ENV) go run ./cmd/flui-server & npm --prefix web run dev; wait

docker:
	docker build -f deploy/docker/Dockerfile --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t $(IMAGE):$(VERSION) .

clean:
	rm -rf bin .dev-data
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete

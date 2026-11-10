# Notes for coding agents

- Plan and docs: user-facing behaviour is documented in `docs/`; keep docs and code in sync in the same change.
- Go 1.27 is installed locally; `go vet ./...` and `go test -count=1 ./...` work directly. If needed, the same tooling runs in Docker:
  - Tests/vet: `docker run --rm -v "$PWD":/src -w /src -v flui-gomod:/go/pkg/mod -v flui-gocache:/root/.cache/go-build golang:1.27.3-alpine sh -c 'go vet ./... && go test -count=1 ./...'`
  - Lint: same mounts with image `golangci/golangci-lint:v2.15.0`, command `golangci-lint run`
  - Vulns: `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` inside `golang:1.27.3-alpine`
- UI: `npm --prefix web ci && npm --prefix web run build` (outputs to `internal/webui/dist`; the build script restores `.gitkeep`).
- Image: `docker build -f deploy/docker/Dockerfile -t flui:dev .`, smoke test `curl -k https://127.0.0.1:8443/readyz`.
- Docs lint: `npx --yes markdownlint-cli2@0.18.1 "**/*.md"`.
- Dependencies: pin exact versions published at least 7 days ago; pin GitHub Actions and base images by SHA/digest.

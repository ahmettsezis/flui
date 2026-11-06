# Build: docker build -f deploy/docker/Dockerfile -t ghcr.io/ahmettsezis/flui .
FROM node:24-alpine@sha256:a34ab6d7f6f4813f4d0c5469d74685604b962fbdbf5793a7bc7b4998ecb867c8 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
RUN npm run build

FROM golang:1.27.3-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/webui/dist/ internal/webui/dist/
ARG VERSION=dev COMMIT=none DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/ahmettsezis/flui/internal/version.Version=${VERSION} -X github.com/ahmettsezis/flui/internal/version.Commit=${COMMIT} -X github.com/ahmettsezis/flui/internal/version.Date=${DATE}" \
      -o /out/flui-server ./cmd/flui-server \
 && install -d -m 0700 /out/data

FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
COPY --from=build /out/flui-server /usr/bin/flui-server
COPY --from=build --chown=65532:65532 /out/data /data
ENV FLUI_DATA_DIR=/data
USER 65532:65532
VOLUME /data
EXPOSE 8443
ENTRYPOINT ["/usr/bin/flui-server"]
CMD ["serve"]

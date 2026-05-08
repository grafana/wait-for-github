FROM --platform=${BUILDPLATFORM} golang:1.26.2-alpine3.23@sha256:f85330846cde1e57ca9ec309382da3b8e6ae3ab943d2739500e08c86393a21b1 AS builder

# Dependencies required to run the race detector
RUN \
  --mount=type=cache,target=/var/cache/apk \
  apk add --no-cache gcc musl-dev

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN \
  --mount=type=cache,target=/go/pkg/mod <<EOF
  go mod download
  go mod verify
EOF

COPY . .

# `go test` requires cgo
RUN \
  --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  CGO_ENABLED=1 go test -race -v ./...

RUN \
  --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  CGO_ENABLED=0 \
  GOOS=${TARGETOS} \
  GOARCH=${TARGETARCH} \
  \
  go \
  build \
  -ldflags="-w -s" \
  -o /go/bin/app \
  github.com/grafana/wait-for-github/cmd/wait-for-github

FROM gcr.io/distroless/static-debian12@sha256:20bc6c0bc4d625a22a8fde3e55f6515709b32055ef8fb9cfbddaa06d1760f838

COPY --from=builder /go/bin/app /go/bin/app

ENTRYPOINT ["/go/bin/app"]

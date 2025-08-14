FROM --platform=${BUILDPLATFORM} golang:1.25.0-alpine3.21@sha256:382d1a77317f5dc0ea2af2c132af50aa579cbc6ad07f8fd2dba08e318adf677b AS builder

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

FROM gcr.io/distroless/static-debian12@sha256:2e114d20aa6371fd271f854aa3d6b2b7d2e70e797bb3ea44fb677afec60db22c

COPY --from=builder /go/bin/app /go/bin/app

ENTRYPOINT ["/go/bin/app"]

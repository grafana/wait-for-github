FROM --platform=${BUILDPLATFORM} golang:1.25.4-alpine3.21@sha256:3289aac2aac769e031d644313d094dbda745f28af81cd7a94137e73eefd58b33 AS builder

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

FROM gcr.io/distroless/static-debian12@sha256:87bce11be0af225e4ca761c40babb06d6d559f5767fbf7dc3c47f0f1a466b92c

COPY --from=builder /go/bin/app /go/bin/app

ENTRYPOINT ["/go/bin/app"]

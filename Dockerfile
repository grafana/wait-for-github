FROM --platform=${BUILDPLATFORM} golang:1.27.3-alpine3.23@sha256:754d5200fc3edae76d3b6598a9a3fac7e74981ee6a5413ab955025f185753ef2 AS builder

# Dependencies required to run the race detector
RUN \
  --mount=type=cache,target=/var/cache/apk <<EOF
  apk add --no-cache gcc musl-dev
  apk add --no-cache 'ca-certificates>=20261020-r0' \
    && update-ca-certificates
EOF

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

FROM gcr.io/distroless/static-debian13@sha256:034df5c1737e8241f6ed6423a12e720ee0f6d6f088609ee172f98f6f2166ef5d

COPY --from=builder /go/bin/app /go/bin/app

ENTRYPOINT ["/go/bin/app"]

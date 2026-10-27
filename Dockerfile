FROM --platform=${BUILDPLATFORM} golang:1.27.3-alpine3.23@sha256:8a5046497cda39860b832ffdbe8dd66018a0f05bb01352f533a36e7e2ced66a0 AS builder

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

FROM gcr.io/distroless/static-debian13@sha256:15f8637509792e05b97d83451b0b6ccfa4abd548f997e406ac1e72c87fe1fa78

COPY --from=builder /go/bin/app /go/bin/app

ENTRYPOINT ["/go/bin/app"]

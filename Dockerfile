FROM --platform=${BUILDPLATFORM} golang:1.24.2-alpine3.21@sha256:7772cb5322baa875edd74705556d08f0eeca7b9c4b5367754ce3f2f00041ccee AS builder

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN <<EOF
  go mod download
  go mod verify
EOF

COPY . .

RUN go get -d -v ./...
RUN CGO_ENABLED=0 go test -v ./...
RUN CGO_ENABLED=0 go build -o /go/bin/app github.com/grafana/wait-for-github/cmd/wait-for-github

FROM gcr.io/distroless/static-debian12@sha256:3d0f463de06b7ddff27684ec3bfd0b54a425149d0f8685308b1fdf297b0265e9

COPY --from=builder /go/bin/app /go/bin/app

ENTRYPOINT ["/go/bin/app"]

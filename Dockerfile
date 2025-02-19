FROM --platform=${BUILDPLATFORM} golang:1.24.0-alpine3.21@sha256:2d40d4fc278dad38be0777d5e2a88a2c6dee51b0b29c97a764fc6c6a11ca893c AS builder

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

FROM gcr.io/distroless/static-debian12@sha256:3f2b64ef97bd285e36132c684e6b2ae8f2723293d09aae046196cca64251acac

COPY --from=builder /go/bin/app /go/bin/app

ENTRYPOINT ["/go/bin/app"]

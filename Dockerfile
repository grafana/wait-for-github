FROM --platform=${BUILDPLATFORM} golang:1.24.1-alpine3.21@sha256:43c094ad24b6ac0546c62193baeb3e6e49ce14d3250845d166c77c25f64b0386 AS builder

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

FROM gcr.io/distroless/static-debian12@sha256:95ea148e8e9edd11cc7f639dc11825f38af86a14e5c7361753c741ceadef2167

COPY --from=builder /go/bin/app /go/bin/app

ENTRYPOINT ["/go/bin/app"]

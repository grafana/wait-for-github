FROM golang:1.20.2-alpine3.17 AS builder

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN CGO_ENABLED=0 go test -v ./...
RUN CGO_ENABLED=0 go build -o /go/bin/app github.com/grafana/wait-for-github/cmd/wait-for-github

FROM gcr.io/distroless/static-debian11

COPY --from=builder /go/bin/app /go/bin/app

ENTRYPOINT ["/go/bin/app"]

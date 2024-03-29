# This is a production multi stage Dockerfile to produce a `distroless` docker image for our production k8s.
# If you wanna try this Dockerfile in your local environment for testing purpose then you can run as follows:
#
# docker build -t metrics-sidecar .
#
# To run the above docker container for testing, you can run following command from your terminal:
#
# docker run -ti --network host metrics-sidecar
#
# On the above `--network host`, we are trying to use host machine's network so that we can use database connectivity
# through `localhost`.
#
# Reference: https://github.com/GoogleContainerTools/distroless/blob/main/base/README.md

FROM golang:1.18 AS builder

COPY go.mod /go/metrics-sidecar/
COPY go.sum /go/metrics-sidecar/
COPY main.go /go/metrics-sidecar/

WORKDIR /go/metrics-sidecar/

RUN go build -ldflags="-s -w" -o bin/metrics-sidecar main.go

FROM gcr.io/distroless/base

LABEL maintainer="Zhang Debo <zhang_debo@c.tre-inc.com>"

COPY --from=builder /go/metrics-sidecar/bin/metrics-sidecar /metrics-sidecar

EXPOSE 9999

CMD ["/metrics-sidecar"]

FROM alpine/git AS git-repo

WORKDIR /workdir
RUN git clone --depth 1 --branch v1.1.0 https://github.com/kubernetes-sigs/gateway-api.git

FROM golang:1.23 AS conformance-test-runner

COPY --from=git-repo /workdir/gateway-api/ /workdir/gateway-api/
WORKDIR /workdir/gateway-api
RUN go mod download

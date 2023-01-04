FROM golang:1.18.3 as build

LABEL maintainer="Calvin0327 <wen.chen@daocloud.io>" \
    maintainer.org="DaoCloud" \
    maintainer.org.uri="https://github.com/DaoCloud"

WORKDIR /workspace
RUN curl -LO https://release.daocloud.io/chartrepo/karmada/charts/karmada-0.0.5.tgz

ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -mod=vendor -o operator ./cmd

FROM alpine:3.15
COPY --from=build /workspace/operator /usr/local/bin/operator

RUN mkdir -p /var/run/karmada-operator/release.daocloud.io/chartrepo/karmada
COPY --from=build /workspace/karmada-0.0.5.tgz /var/run/karmada-operator/release.daocloud.io/chartrepo/karmada/karmada-0.0.5.tgz

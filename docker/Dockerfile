FROM golang:1.18.3 as build

LABEL maintainer="Calvin0327 <wen.chen@daocloud.io>" \
    maintainer.org="DaoCloud" \
    maintainer.org.uri="https://github.com/DaoCloud"

WORKDIR /workspace

ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) && echo "Building GoARCH of &GOARCH..." \
    && go build -mod=vendor -o operator ./cmd

FROM gcr.io/distroless/base:nonroot
WORKDIR /
COPY --from=build /workspace/operator /usr/local/bin/operator
USER 65532:65532

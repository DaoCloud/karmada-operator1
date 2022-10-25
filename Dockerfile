FROM golang:1.18.3 as build

WORKDIR /workspace

ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) && echo "Building GoARCH of &GOARCH..." \
     && go build -mod=vendor -o operator ./cmd

FROM alpine:3.15

COPY --from=build /workspace/operator /usr/local/bin/operator

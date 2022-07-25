FROM golang:1.18.3 as build

WORKDIR /workspace

ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct

COPY . .


RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor  -o manager ./cmd/controller-manager

FROM alpine:3.15

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

ENV TZ=Asia/Shanghai
COPY --from=build /workspace/manager .

ENTRYPOINT ["/manager"]
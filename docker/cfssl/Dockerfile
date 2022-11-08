FROM golang:1.16.15

LABEL maintainer="Calvin0327 <wen.chen@daocloud.io>" \
     maintainer.org="DaoCloud" \
     maintainer.org.uri="https://github.com/DaoCloud"

WORKDIR /workspace

RUN git clone --single-branch --depth 1 --branch master https://github.com/cloudflare/cfssl_trust.git /etc/cfssl && \
     git clone --single-branch --depth 1 --branch master https://github.com/cloudflare/cfssl.git /workspace && \
     make clean && \
     make all && \
     mv bin/* /usr/bin/ && \
     rm /workspace/* -rf

EXPOSE 8888

ENTRYPOINT ["cfssl"]
CMD ["--help"]

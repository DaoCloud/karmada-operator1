FROM docker.io/bitnami/minideb:bullseye

LABEL maintainer="Calvin0327 <wen.chen@daocloud.io>" \
     maintainer.org="DaoCloud" \
     maintainer.org.uri="https://github.com/DaoCloud"

ARG KUBECTL_VERSION="v1.25.3"

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# Install required system packages and dependencies
RUN apt-get update -qq && \
     apt-get install -y --no-install-recommends ca-certificates curl git jq procps && \
     apt-get clean && rm -rf /var/lib/apt/lists /var/cache/apt/archives

RUN mkdir -p /tmp/karmada-operator/pkg/cache/ && cd /tmp/karmada-operator/pkg/cache/ && \
     ARCH="$(uname -m | sed -e 's/x86_64/amd64/' -e 's/\(arm\)\(64\)\?.*/\1\2/' -e 's/aarch64$/arm64/')" && \
     echo "Building ARCH of $ARCH.." && \
     curl -sLO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl" -o kubectl && \
     chmod +x kubectl && \ 
     mv kubectl /usr/local/bin/

RUN apt-get autoremove --purge -y curl && \
     apt-get update && apt-get upgrade -y && \
     apt-get clean && rm -rf /var/lib/apt/lists /var/cache/apt/archives

RUN mkdir /.kube && chmod g+rwX /.kube

ENV APP_VERSION="1.25.3" \
     OPERATOR_APP_NAME="kubectl"

USER 1001
ENTRYPOINT [ "kubectl" ]
CMD [ "--help" ]

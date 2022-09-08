GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
SOURCES := $(shell find ./cmd/controller-manager -type f  -name '*.go')

BUILD_ARCH ?= linux/$(GOARCH)

# Git information
GIT_VERSION ?= $(shell git describe --tags --abbrev=8 --dirty) # attention: gitlab CI: git fetch should not use shallow
GIT_COMMIT_HASH ?= $(shell git rev-parse HEAD)
GIT_TREESTATE = "clean"
GIT_DIFF = $(shell git diff --quiet >/dev/null 2>&1; if [ $$? -eq 1 ]; then echo "1"; fi)
ifeq ($(GIT_DIFF), 1)
    GIT_TREESTATE = "dirty"
endif
BUILDDATE = $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := "-X github.com/daocloud/karmada-operator/pkg/version.gitVersion=$(GIT_VERSION) \
            -X github.com/daocloud/karmada-operator/pkg/version.gitCommit=$(GIT_COMMIT_HASH) \
            -X github.com/daocloud/karmada-operator/pkg/version.gitTreeState=$(GIT_TREESTATE) \
            -X github.com/daocloud/karmada-operator/pkg/version.buildDate=$(BUILDDATE)"

GOBIN         = $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN         = $(shell go env GOPATH)/bin
endif
GOIMPORTS     = $(GOBIN)/goimports

# Images management
REGISTRY_SERVER_ADDRESS?=release.daocloud.io
REGISTRY_REPO?=$(REGISTRY_SERVER_ADDRESS)/karmada
HELM_REPO?="https://$(REGISTRY_SERVER_ADDRESS)/chartrepo/karmada-operator"
#If you have not logged in to the registry, you need to fill in the account and password
REGISTRY_USER_NAME?=""
REGISTRY_PASSWORD?=""
#The kubeconfig path of the cluster where karmada-operator deploy
YOUR_KUBE_CONF?="/root/.kube/config"

# Parameter
KARMADA_OPERATOR_NAMESPACE="karmada-operator-system"

DEPLOY_ENV?="PROD"

# Set your version by env or using latest tags from git
VERSION?=""
ifeq ($(VERSION), "")
    LATEST_TAG=$(shell git describe --tags --abbrev=8)
    ifeq ($(LATEST_TAG),)
        # Forked repo may not sync tags from upstream, so give it a default tag to make CI happy.
        VERSION="v0.1.0"
    else
        VERSION=$(LATEST_TAG)
    endif
endif

BENCKMARK_THRESHOLD?="50"

# convert to git version to semver version v0.1.1-14-gb943a40 --> v0.1.1+14-gb943a40
KARMADA_OPERATOR_VERSION := $(shell echo $(VERSION) | sed 's/-/+/1')

# convert to git version to semver version v0.1.1+14-gb943a40 --> v0.1.1-14-gb943a40
KARMADA_OPERATOR_IMAGE_VERSION := $(shell echo $(KARMADA_OPERATOR_VERSION) | sed 's/+/-/1')

#v0.1.1 --> 0.1.1 Match the helm chart version specification, remove the preceding prefix `v` character
KARMADA_OPERATOR_CHART_VERSION := $(shell echo ${KARMADA_OPERATOR_VERSION} |sed  's/^v//g' )

.PHONY: karmada-operator-imgs
karmada-operator-imgs: karmada-operator

all: karmada-operator-imgs

.PHONY: karmada-operator
karmada-operator: $(SOURCES)
	echo "Building karmada-operator for arch = $(BUILD_ARCH)"
	export DOCKER_CLI_EXPERIMENTAL=enabled ;\
	! ( docker buildx ls | grep karmada-operator-builder ) && docker buildx create --use --platform=$(BUILD_ARCH) --name karmada-operator-builder ;\
	docker buildx build \
		--build-arg karmada-operator_version=$(KARMADA_OPERATOR_VERSION) \
		--build-arg UBUNTU_MIRROR=$(UBUNTU_MIRROR) \
		--builder karmada-operator-builder \
		--platform $(BUILD_ARCH) \
		--tag $(REGISTRY_REPO)/karmada-operator:$(KARMADA_OPERATOR_IMAGE_VERSION)  \
		--tag $(REGISTRY_REPO)/karmada-operator:latest  \
		-f ./Dockerfile \
		--load \
		.

.PHONY: upload-image
upload-image: karmada-operator-imgs
	@echo "push images to $(REGISTRY_REPO)"
	@docker push $(REGISTRY_REPO)/karmada-operator:$(KARMADA_OPERATOR_IMAGE_VERSION)
	@docker push $(REGISTRY_REPO)/karmada-operator:latest

.PHONY: update-code-gen
update-code-gen:
	./hack/update-codegen.sh

.PHONY: update-crds
update-crds:
	./hack/update-crds.sh

.PHONY: push-chart
push-chart:
	#helm package -u ./charts/ -d ./dist/
	helm repo add karmada-operator-release $(HELM_REPO)
	helm dependency update ./charts/karmada-operator
	helm package ./charts/karmada-operator -d dist --version $(KARMADA_OPERATOR_CHART_VERSION)
	helm cm-push ./dist/karmada-operator-$(KARMADA_OPERATOR_CHART_VERSION).tgz  karmada-operator-release -a $(KARMADA_OPERATOR_CHART_VERSION) -v $(KARMADA_OPERATOR_CHART_VERSION) -u $(REGISTRY_USER_NAME)  -p $(REGISTRY_PASSWORD)

.PHONY: clean-chart
clean-chart:
	rm -rf  dist

.PHONY: release
release: karmada-operator-imgs upload-image push-chart

## Deploy current version of helm package to target cluster of $(YOUR_KUBE_CONF) [not defined]
.PHONY: deploy
deploy:
	bash hack/deploy.sh  "$(KARMADA_OPERATOR_CHART_VERSION)" "$(KARMADA_OPERATOR_IMAGE_VERSION)"  "$(YOUR_KUBE_CONF)" "$(KARMADA_OPERATOR_NAMESPACE)" "$(HELM_REPO)" "$(REGISTRY_SERVER_ADDRESS)" "$(DEPLOY_ENV)"

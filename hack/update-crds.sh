#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT=$(git rev-parse --show-toplevel)

API_ROOT="${REPO_ROOT}/pkg/apis/"

echo "Generating CRDs With controller-gen"
GO11MODULE=on go install sigs.k8s.io/controller-tools/cmd/controller-gen

cd "${API_ROOT}"
controller-gen crd paths=./install/v1alpha1/... output:crd:dir="${REPO_ROOT}/charts/karmada-operator/_crds"

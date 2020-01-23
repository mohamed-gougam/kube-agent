#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd "${SCRIPT_ROOT}"; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../code-generator)}

echo script_root=$SCRIPT_ROOT
echo codegen_pkg=$CODEGEN_PKG

${CODEGEN_PKG}/generate-groups.sh all \
  github.com/mohamed-gougam/kube-agent/pkg/client github.com/mohamed-gougam/kube-agent/pkg/apis \
  k8snginx:v1 \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt

#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

PACKAGE_LLM_D_SIM="github.com/llm-d/llm-d-sim/cmd/llm-d-sim"
LLM_D_SIM_NAME="llm-d-sim"
LLM_D_SIM_TAG="v0.0.1"

go get --tool ${PACKAGE_LLM_D_SIM}
LLM_D_SIM_DIR=$(go list -f '{{.Dir}}' -m github.com/llm-d/llm-d-sim)

GOOS=linux GOARCH=amd64 go build -o bin/linux/ ${PACKAGE_LLM_D_SIM}
docker build --file ${LLM_D_SIM_DIR}/build/llm-d-sim.Dockerfile --tag ${LLM_D_SIM_NAME}:${LLM_D_SIM_TAG} ./bin/linux

go get --tool ${PACKAGE_LLM_D_SIM}@none # remove dependency
go mod tidy

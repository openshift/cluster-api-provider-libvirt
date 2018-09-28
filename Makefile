# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

.PHONY: all
all: build images check

INSTALL_DEPS ?= 0

.PHONY: depend
depend:
	dep version || go get -u github.com/golang/dep/cmd/dep
	dep ensure

.PHONY: depend-update
depend-update:
	dep ensure -update

.PHONY: deps-cgo
deps-cgo:
	@if [ $(INSTALL_DEPS) == 1 ]; then yum install -y libvirt-devel; fi

build: deps-cgo ## build binary
	CGO_ENABLED=1 go install sigs.k8s.io/cluster-api-provider-libvirt/cmd/machine-controller

.PHONY: images
images: ## Create images
	$(MAKE) -C cmd/machine-controller image

.PHONY: push
push:
	$(MAKE) -C cmd/machine-controller push

.PHONY: check
check: fmt vet lint test ## Check your code

.PHONY: test
test: # Run unit test
	go test -race -cover ./cmd/... ./cloud/...

.PHONY: integration
integration: deps-cgo ## Run integration test
	go test -v sigs.k8s.io/cluster-api-provider-libvirt/test/integration

.PHONY: e2e
e2e: deps-cgo ## Run end-to-end test
	hack/packet-provision.sh install
	#TODO run tests
	hack/packet-provision.sh destroy

.PHONY: lint
lint: ## Go lint your code
	hack/go-lint.sh -min_confidence 0.3 $(go list -f '{{ .ImportPath }}' ./...)

.PHONY: fmt
fmt: ## Go fmt your code
	hack/go-fmt.sh

.PHONY: vet
vet: ## Apply go vet to all go files
	hack/go-vet.sh ./...

.PHONY: help
help:
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

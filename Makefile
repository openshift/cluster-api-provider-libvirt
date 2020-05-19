DBG         ?= 0
VERSION     ?= $(shell git describe --always --abbrev=7)
MUTABLE_TAG ?= latest
PROJECT     ?= cluster-api-provider-libvirt
ORG_PATH    ?= github.com/openshift
REPO_PATH   ?= $(ORG_PATH)/$(PROJECT)
CLUSTER_API ?= github.com/openshift/cluster-api
IMAGE        = origin-libvirt-machine-controllers

ifeq ($(DBG),1)
GOGCFLAGS ?= -gcflags=all="-N -l"
endif

.PHONY: all
all: build images check

NO_DOCKER ?= 0
ifeq ($(NO_DOCKER), 1)
  DOCKER_CMD =
  IMAGE_BUILD_CMD = imagebuilder
  CGO_ENABLED = 1
else
  DOCKER_CMD := docker run --rm -e CGO_ENABLED=1 -v "$(PWD):/go/src/$(REPO_PATH):Z" -w "/go/src/$(REPO_PATH)" openshift/origin-release:golang-1.10
  IMAGE_BUILD_CMD = docker build
endif

.PHONY: depend-update
depend-update:
	GO111MODULE=on go mod tidy
	GO111MODULE=on go mod vendor

.PHONY: generate
generate: gendeepcopy gencode

.PHONY: gencode
gencode:
	go install $(GOGCFLAGS) -ldflags '-extldflags "-static"' github.com/openshift/cluster-api-provider-libvirt/vendor/github.com/golang/mock/mockgen
	go generate ./pkg/... ./cmd/...

.PHONY: gendeepcopy
gendeepcopy:
	go build -o $$GOPATH/bin/deepcopy-gen "$(REPO_PATH)/vendor/k8s.io/code-generator/cmd/deepcopy-gen"
	deepcopy-gen \
          -i ./pkg/apis/libvirtproviderconfig,./pkg/apis/libvirtproviderconfig/v1beta1 \
          -O zz_generated.deepcopy \
          -h hack/boilerplate.go.txt

.PHONY: build
build: ## build binaries
	$(DOCKER_CMD) go build $(GOGCFLAGS) -o bin/machine-controller "$(REPO_PATH)/cmd/manager"
	$(DOCKER_CMD) go test $(GOGCFLAGS) -c -o bin/machines.test "$(REPO_PATH)/test/machines"

.PHONY: libvirt-actuator
libvirt-actuator:
	$(DOCKER_CMD) go build $(GOGCFLAGS) -o bin/libvirt-actuator "$(REPO_PATH)/cmd/libvirt-actuator"

.PHONY: images
images: ## Create images
	$(IMAGE_BUILD_CMD) -t "$(IMAGE):$(VERSION)" -t "$(IMAGE):$(MUTABLE_TAG)" ./

.PHONY: push
push:
	docker push "$(IMAGE):$(VERSION)"
	docker push "$(IMAGE):$(MUTABLE_TAG)"

.PHONY: check
check: fmt vet lint test check-pkg ## Check your code

.PHONY: check-pkg
check-pkg:
	./hack/verify-actuator-pkg.sh

.PHONY: test
test: # Run unit test
	$(DOCKER_CMD) go test -race -cover ./cmd/... ./pkg/cloud/...

.PHONY: build-e2e
build-e2e:
	$(DOCKER_CMD) go test -c -o bin/machines.test "$(REPO_PATH)/test/machines"

.PHONY: test-e2e
test-e2e: images build-e2e e2e-provision ## Run end-to-end test
	hack/test-e2e.sh || ($(MAKE) e2e-clean && false)
	$(MAKE) e2e-clean

.PHONY: e2e-provision
e2e-provision:
	hack/packet-provision.sh install

.PHONY: e2e-clean
e2e-clean:
	hack/packet-provision.sh destroy

.PHONY: lint
lint: ## Go lint your code
	hack/go-lint.sh -min_confidence 0.3 $(go list -f '{{ .ImportPath }}' ./...)

.PHONY: fmt
fmt: ## Go fmt your code
	hack/go-fmt.sh .

.PHONY: vet
vet: ## Apply go vet to all go files
	hack/go-vet.sh ./...

.PHONY: help
help:
	@grep -E '^[a-zA-Z/0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

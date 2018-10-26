DBG         ?= 0
VERSION     ?= $(shell git describe --always --abbrev=7)
MUTABLE_TAG ?= latest
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
  DOCKER_CMD := docker run --rm -e CGO_ENABLED=1 -v "$(PWD)":/go/src/github.com/openshift/cluster-api-provider-libvirt:Z -w /go/src/github.com/openshift/cluster-api-provider-libvirt openshift/origin-release:golang-1.10
  IMAGE_BUILD_CMD = docker build
endif

.PHONY: depend
depend:
	dep version || go get -u github.com/golang/dep/cmd/dep
	dep ensure

.PHONY: depend-update
depend-update:
	dep ensure -update

.PHONY: vendor
vendor:
	dep version || go get -u github.com/golang/dep/cmd/dep
	dep ensure -v -update
	patch -p1 < 0001-Delete-annotated-machines-first-when-scaling-down.patch
	patch -p1 < 0002-Sort-machines-before-syncing.patch
	patch -p1 < 0001-use-Update-instead-of-Status.Update-as-CustomResourc.patch

.PHONY: generate
generate: gendeepcopy

.PHONY: gendeepcopy
gendeepcopy:
	go build -o $$GOPATH/bin/deepcopy-gen github.com/openshift/cluster-api-provider-libvirt/vendor/k8s.io/code-generator/cmd/deepcopy-gen
	deepcopy-gen \
          -i ./pkg/apis/libvirtproviderconfig,./pkg/apis/libvirtproviderconfig/v1alpha1 \
          -O zz_generated.deepcopy \
          -h hack/boilerplate.go.txt

.PHONY: build
build: ## build binaries
	$(DOCKER_CMD) go build $(GOGCFLAGS) -o bin/machine-controller github.com/openshift/cluster-api-provider-libvirt/cmd/manager
	$(DOCKER_CMD) go test $(GOGCFLAGS) -c -o bin/machines.test github.com/openshift/cluster-api-provider-libvirt/test/machines

.PHONY: libvirt-actuator
libvirt-actuator:
	$(DOCKER_CMD) go build $(GOGCFLAGS) -o bin/libvirt-actuator github.com/openshift/cluster-api-provider-libvirt/cmd/libvirt-actuator

.PHONY: images
images: ## Create images
	$(IMAGE_BUILD_CMD) -t "$(IMAGE):$(VERSION)" -t "$(IMAGE):$(MUTABLE_TAG)" ./

.PHONY: push
push:
	docker push "$(IMAGE):$(VERSION)"
	docker push "$(IMAGE):$(MUTABLE_TAG)"

.PHONY: check
check: fmt vet lint test ## Check your code

.PHONY: test
test: # Run unit test
	$(DOCKER_CMD) go test -race -cover ./cmd/... ./cloud/...

.PHONY: integration
integration: deps-cgo ## Run integration test
	$(DOCKER_CMD) go test -v sigs.k8s.io/cluster-api-provider-libvirt/test/integration

.PHONY: e2e
e2e: e2e-provision ## Run end-to-end test
	# TODO @ingvagabund @spangenberg add e2e test command here
	hack/packet-provision.sh destroy

.PHONY: e2e-provision
e2e-provision:
	hack/packet-provision.sh install

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
	@grep -E '^[a-zA-Z/0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

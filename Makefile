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

.PHONY: gendeepcopy

all: generate build images

INSTALL_DEPS ?= 0

depend:
	dep version || go get -u github.com/golang/dep/cmd/dep
	dep ensure

depend-update:
	dep ensure -update

deps-cgo:
	@if [ $(INSTALL_DEPS) == 1 ]; then yum install -y libvirt-devel; fi

generate: gendeepcopy

gendeepcopy:
	go build -o $$GOPATH/bin/deepcopy-gen github.com/enxebre/cluster-api-provider-libvirt/vendor/k8s.io/code-generator/cmd/deepcopy-gen
	deepcopy-gen \
	  -i ./cloud/libvirt/providerconfig,./cloud/libvirt/providerconfig/v1alpha1 \
	  -O zz_generated.deepcopy \
	  -h boilerplate.go.txt

build: deps-cgo
	CGO_ENABLED=1 go install sigs.k8s.io/cluster-api-provider-libvirt/cmd/machine-controller

images:
	$(MAKE) -C cmd/machine-controller image

push:
	$(MAKE) -C cmd/machine-controller push

check: fmt vet

test: deps-cgo
	go test -race -cover ./cmd/... ./cloud/...

integration: deps-cgo
	go test -v sigs.k8s.io/cluster-api-provider-libvirt/test/integration

fmt:
	hack/go-fmt.sh .

vet:
	hack/go-vet.sh ./...

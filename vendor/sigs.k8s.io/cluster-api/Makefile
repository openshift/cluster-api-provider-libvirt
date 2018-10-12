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

# Image URL to use all building/pushing image targets
IMG ?= gcr.io/k8s-cluster-api/cluster-api-controller:latest

all: test manager clusterctl

# Run tests
test: generate fmt vet manifests verify
	go test -v -tags=integration ./pkg/... ./cmd/...
#	go test -v -tags=integration ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager sigs.k8s.io/cluster-api/cmd/manager

# Build manager binary
clusterctl: generate fmt vet
	go build -o bin/clusterctl sigs.k8s.io/cluster-api/cmd/clusterctl

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

<<<<<<< HEAD
genapi: depend
	go build -o $$GOPATH/bin/apiregister-gen sigs.k8s.io/cluster-api/vendor/github.com/kubernetes-incubator/apiserver-builder/cmd/apiregister-gen
	$$GOPATH/bin/apiregister-gen -i ./pkg/apis,./pkg/apis/cluster,./pkg/apis/cluster/v1alpha1

genconversion: depend
	go build -o $$GOPATH/bin/conversion-gen sigs.k8s.io/cluster-api/vendor/k8s.io/code-generator/cmd/conversion-gen
	$$GOPATH/bin/conversion-gen -i ./pkg/apis/cluster/v1alpha1/ -O zz_generated.conversion --go-header-file boilerplate.go.txt

genclientset: depend
	go build -o $$GOPATH/bin/client-gen sigs.k8s.io/cluster-api/vendor/k8s.io/code-generator/cmd/client-gen
	$$GOPATH/bin/client-gen \
	  --input="cluster/v1alpha1" \
		--clientset-name="clientset" \
		--input-base="sigs.k8s.io/cluster-api/pkg/apis" \
		--output-package "sigs.k8s.io/cluster-api/pkg/client/clientset_generated" \
		--go-header-file boilerplate.go.txt \
		--clientset-path sigs.k8s.io/cluster-api/pkg/client/clientset_generated

gendeepcopy:
	go build -o $$GOPATH/bin/deepcopy-gen sigs.k8s.io/cluster-api/vendor/k8s.io/code-generator/cmd/deepcopy-gen
	$$GOPATH/bin/deepcopy-gen \
	  -i ./pkg/apis/cluster/,./pkg/apis/cluster/v1alpha1/ \
	  -O zz_generated.deepcopy \
	  -h boilerplate.go.txt
=======
# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...
>>>>>>> Re-Vendoring

# Generate code
generate:
	go generate ./pkg/... ./cmd/...

<<<<<<< HEAD
# Automatically extract vendored apis under vendor/k8s.io/api.
VENDOR_API_DIRS := $(shell find vendor/k8s.io/api -type d | grep -E 'v[[:digit:]]+(alpha[[:digit:]]+|beta[[:digit:]]+)*' | sed -e 's/^vendor\///')

empty:=
comma:=,
space:=$(empty) $(empty)

genopenapi: static_apis = $(subst $(space),$(comma),$(STATIC_API_DIRS))
genopenapi: vendor_apis = $(subst $(space),$(comma),$(VENDOR_API_DIRS))

genopenapi:
	go build -o $$GOPATH/bin/openapi-gen sigs.k8s.io/cluster-api/vendor/k8s.io/code-generator/cmd/openapi-gen
	$$GOPATH/bin/openapi-gen \
	  --input-dirs $(static_apis) \
	  --input-dirs $(vendor_apis) \
	  --input-dirs ./pkg/apis/cluster/,./pkg/apis/cluster/v1alpha1/ \
	  --output-package "sigs.k8s.io/cluster-api/pkg/openapi" \
	  --go-header-file boilerplate.go.txt

build: depend
	CGO_ENABLED=0 go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/cmd/apiserver
	CGO_ENABLED=0 go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/cmd/controller-manager

images: depend
	$(MAKE) -C cmd/apiserver image
	$(MAKE) -C cmd/controller-manager image
=======
# Build the docker image
docker-build: generate fmt vet manifests
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml
>>>>>>> Re-Vendoring

verify:
	./hack/verify_boilerplate.py

# Push the docker image
docker-push:
	docker push ${IMG}

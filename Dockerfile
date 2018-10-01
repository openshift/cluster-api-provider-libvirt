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

# Reproducible builder image
FROM openshift/origin-release:golang-1.10 as builder

# Workaround a bug in imagebuilder (some versions) where this dir will not be auto-created.
RUN mkdir -p /go/src/github.com/openshift/cluster-api-provider-libvirt
WORKDIR /go/src/github.com/openshift/cluster-api-provider-libvirt

# This expects that the context passed to the docker build command is
# the cluster-api-provider-libvirt directory.
# e.g. docker build -t <tag> -f <this_Dockerfile> <path_to_cluster-api-libvirt>
COPY . .
RUN yum install -y libvirt-devel
RUN GOPATH=/go CGO_ENABLED=1 go install ./cmd/machine-controller
RUN GOPATH=/go CGO_ENABLED=0 GOOS=linux go install -a -ldflags '-extldflags "-static"' github.com/openshift/cluster-api-provider-libvirt/vendor/sigs.k8s.io/cluster-api/cmd/controller-manager
RUN GOPATH=/go CGO_ENABLED=0 GOOS=linux go install -a -ldflags '-extldflags "-static"' github.com/openshift/cluster-api-provider-libvirt/vendor/sigs.k8s.io/cluster-api/cmd/apiserver

# Final container
FROM openshift/origin-base
RUN yum install -y ca-certificates libvirt-libs openssh

COPY --from=builder /go/bin/machine-controller /go/bin/controller-manager .

# Reproducible builder image
FROM openshift/origin-release:golang-1.10 as builder

# Workaround a bug in imagebuilder (some versions) where this dir will not be auto-created.
RUN mkdir -p /go/src/github.com/openshift/cluster-api-provider-libvirt
WORKDIR /go/src/github.com/openshift/cluster-api-provider-libvirt

# This expects that the context passed to the docker build command is
# the cluster-api-provider-libvirt directory.
# e.g. docker build -t <tag> -f <this_Dockerfile> <path_to_cluster-api-libvirt>
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/
COPY lib/ lib/

RUN GOPATH="/go" CGO_ENABLED=1 GOOS=linux go build -o /go/bin/machine-controller-manager github.com/openshift/cluster-api-provider-libvirt/cmd/manager
RUN GOPATH="/go" CGO_ENABLED=0 GOOS=linux go build -o /go/bin/manager -ldflags '-extldflags "-static"' github.com/openshift/cluster-api-provider-libvirt/vendor/sigs.k8s.io/cluster-api/cmd/manager

# Final container
FROM openshift/origin-base
RUN yum install -y ca-certificates libvirt-libs openssh-clients genisoimage

COPY --from=builder /go/bin/manager /go/bin/machine-controller-manager /

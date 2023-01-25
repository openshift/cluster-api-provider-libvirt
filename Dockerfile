FROM registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.19-openshift-4.12 AS builder
WORKDIR /go/src/github.com/openshift/cluster-api-provider-libvirt
COPY . .
RUN go build -o machine-controller-manager ./cmd/manager

FROM quay.io/centos/centos:stream9
RUN INSTALL_PKGS=" \
      libvirt-libs openssh-clients xorriso \
      " && \
    yum install -y $INSTALL_PKGS && \
    rpm -V $INSTALL_PKGS && \
    yum clean all
COPY --from=builder /go/src/github.com/openshift/cluster-api-provider-libvirt/machine-controller-manager /

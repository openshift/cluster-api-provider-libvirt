#!/bin/sh

CONTAINER_RUNTIME=${CONTAINER_RUNTIME:-podman}

if [ "$IS_CONTAINER" != "" ]; then
  go vet "${@}"
else
  "$CONTAINER_RUNTIME" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/openshift/cluster-api-provider-libvirt:z" \
    --workdir /go/src/github.com/openshift/cluster-api-provider-libvirt \
    registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.19-openshift-4.13 \
    ./hack/go-vet.sh "${@}"
fi;

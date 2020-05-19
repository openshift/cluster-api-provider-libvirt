#!/bin/sh
# Example:  ./hack/go-lint.sh installer/... pkg/... tests/smoke

CONTAINER_RUNTIME=${CONTAINER_RUNTIME:-podman}

if [ "$IS_CONTAINER" != "" ]; then
  golint -set_exit_status "${@}"
else
  "$CONTAINER_RUNTIME" run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/openshift/cluster-api-provider-libvirt:z" \
    --workdir /go/src/github.com/openshift/cluster-api-provider-libvirt \
    openshift/origin-release:golang-1.13 \
    ./hack/go-lint.sh "${@}"
fi

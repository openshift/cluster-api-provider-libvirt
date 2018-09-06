#!/bin/sh
# Example:  ./hack/go-lint.sh installer/... pkg/... tests/smoke

if [ "$IS_CONTAINER" != "" ]; then
  golint -set_exit_status "${@}"
else
  docker run --rm \
    --env IS_CONTAINER=TRUE \
    --volume "${PWD}:/go/src/github.com/openshift/cluster-api-provider-libvirt:z" \
    --workdir /go/src/github.com/openshift/cluster-api-provider-libvirt \
    --entrypoint sh \
    quay.io/coreos/golang-testing \
    ./hack/go-lint.sh "${@}"
fi

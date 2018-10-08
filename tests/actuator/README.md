# Test for actuator in isolation (no cluster api)

## Build
```
CGO_ENABLED=1 go install github.com/openshift/cluster-api-provider-libvirt/tests/actuator
```

## Run
```
$GOPATH/go/bin/actuator -m examples/
```
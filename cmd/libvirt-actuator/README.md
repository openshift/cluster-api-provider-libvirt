# Libvirt actuator

The command allows to directly interact with the libvirt actuator.

## To build the `libvirt-actuator` binary:

You'll need to install libvirt-dev installed on the system you are building and running the binary. e.g. `apt-get -y install libvirt-dev`
```sh
CGO_ENABLED=1 go build -o bin/libvirt-actuator -a github.com/enxebre/cluster-api-provider-libvirt/cmd/libvirt-actuator
```

## Create libvirt instance based on machine manifest

```sh
$ ./bin/libvirt-actuator create -m examples/machine.yaml -c examples/cluster.yaml
```

Once the libvirt instance is created you can run `$ cat /tmp/test` to verify it contains the `Ahoj` string.

## Test if libvirt instance exists based on machine manifest

```sh
$ ./bin/libvirt-actuator exists -m examples/machine.yaml -c examples/cluster.yaml
```

## Delete libvirt instance based on machine manifest

```sh
$ ./bin/libvirt-actuator delete -m examples/machine.yaml -c examples/cluster.yaml
```

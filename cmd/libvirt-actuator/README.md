# Libvirt actuator

The command allows to directly interact with the libvirt actuator.

## How to prepare environment

For running the libvirt actuator with Fedora 28 Cloud images.

1. Provision instance in `packet.net`:
   ```sh
   $ ./hack/packet-provision.sh install
   ```
   and get the IP address (assuming it is `147.75.96.139`).

1. Update ``~/.ssh/config`` to contain:
   ```
   Host libvirtactuator 147.75.96.139
   Hostname 147.75.96.139
   ControlPersist 10m
   ControlMaster auto
   ControlPath /tmp/%r@%h:%p
   User root
   StrictHostKeyChecking no
   PasswordAuthentication no
   UserKnownHostsFile ~/.ssh/aws_known_hosts
   IdentityFile /tmp/packet_id_rsa
   IdentitiesOnly yes
   Compression yes
   ```

1. Run [init script](resources/init.sh) to upload private SSH key to access created guests:

   ```sh
   $ bash cmd/libvirt-actuator/resources/init.sh
   ```

## How to prepare machine resources

By default, available user data (under [user-data.sh](resources/user-data.sh) file)
contains a shell script that deploys kubernetes master node.
Feel free to modify the file to your needs.

The libvirt actuator expects the user data to be provided by a kubernetes secret
by setting `spec.providerConfig.value.cloudInit.userDataSecret` field.
See [userdata.yml](resources/userdata.yml) for example.

At the same time, the `spec.providerConfig.value.uri` needs to be set to libvirt
uri. E.g. `qemu+ssh://root@147.75.96.139/system` in case the libivrt instance
is accessible via ssh on `147.75.96.139` IP address.

## To build the `libvirt-actuator` binary:

You'll need to install `libvirt-dev[el]` installed on the system you are building and running the binary.
e.g. `apt-get -y install libvirt-dev` or `yum -y install libvirt-devel`

```sh
CGO_ENABLED=1 go build -o bin/libvirt-actuator -a github.com/openshift/cluster-api-provider-libvirt/cmd/libvirt-actuator
```

### Create libvirt instance based on machine manifest

```sh
$ ./bin/libvirt-actuator create -m cmd/libvirt-actuator/resources/machine.yaml -c examples/cluster.yaml -u cmd/libvirt-actuator/resources/userdata.yaml
```

Once the libvirt instance is created you can ssh inside.
First, run `domifaddr` to get the guest IP address:
```
$ virsh -c qemu+ssh://root@147.75.96.139/system domifaddr worker-example
```

Then SSH inside using the [guest.pem](resources/guest.pem) (from withing the libvirt instance).

Meantime you can check `/root/user-data.logs` to see the progress of deploying kubernetes master node:
```
$ watch -n 1 sudo tail -20 /root/user-data.logs
```

Once the deployment is done, you can list the master node:

```
$ sudo kubectl get nodes
NAME                   STATUS    ROLES     AGE       VERSION
fedora.example.local   Ready     master    17m       v1.11.3
```

### Test if libvirt instance exists based on machine manifest

```sh
$ ./bin/libvirt-actuator exists -m examples/machine.yaml -c examples/cluster.yaml
```

### Delete libvirt instance based on machine manifest

```sh
$ ./bin/libvirt-actuator delete -m examples/machine.yaml -c examples/cluster.yaml
```

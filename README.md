# OpenShift cluster-api-provider-libvirt

This repository hosts an implementation of a provider for libvirt for the
OpenShift [machine-api](https://github.com/openshift/cluster-api).

This provider runs as a machine-controller deployed by the
[machine-api-operator](https://github.com/openshift/machine-api-operator)

## Allowing the actuator to connect to the libvirt daemon running on the host machine:

Libvirt needs to be configured to accept TCP connections as described in the [installer documentation](https://github.com/openshift/installer/tree/master/docs/dev/libvirt#configure-libvirt-to-accept-tcp-connections).

You can verify that you can connect through your host private ip with:

```sh
virsh -c qemu+tcp://host_private_ip/system
```

## Video demo
https://youtu.be/urvXXfdfzVc

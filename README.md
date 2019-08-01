# OpenShift cluster-api-provider-libvirt

This repository hosts an implementation of a provider for libvirt for the
OpenShift [machine-api](https://github.com/openshift/cluster-api).

This provider runs as a machine-controller deployed by the
[machine-api-operator](https://github.com/openshift/machine-api-operator)

## Allowing the actuator to connect to the libvirt daemon running on the host machine:

Edit `/etc/libvirt/libvirtd.conf` to set:

```
listen_tls = 0
listen_tcp = 1
auth_tcp="none"
tcp_port = "16509"
```

Edit `/etc/systemd/system/libvirt-bin.service` to set:

```
/usr/sbin/libvirtd -l
```

Then:

```
systemctl restart libvirtd
```

Allow incoming connections:

```
iptables -I INPUT -p tcp --dport 16509 -j ACCEPT -m comment --comment "Allow insecure libvirt clients"
```

Verify you can connect through your host private ip:

```
virsh -c qemu+tcp://host_private_ip/system
```

## Run it with the installer
Before running the installer make sure you set libvirt to use the host private ip uri above:
https://github.com/openshift/installer/blob/master/examples/tectonic.libvirt.yaml#L14

## Video demo
https://youtu.be/urvXXfdfzVc

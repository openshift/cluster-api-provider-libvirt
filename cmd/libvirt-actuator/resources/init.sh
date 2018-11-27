#!/bin/sh

export LIBVIRT_DEFAULT_URI=qemu+ssh://root@libvirtactuator/system

script_dir="$(cd $(dirname "${BASH_SOURCE[0]}") && pwd -P)"
scp ${script_dir}/guest.pem libvirtactuator:/root/.ssh/id_rsa
ssh libvirtactuator 'chmod 600 /root/.ssh/id_rsa'

ssh libvirtactuator 'virsh vol-create-as default fedora_base $(stat -Lc%s /Fedora-Cloud-Base-28-1.1.x86_64.qcow2) --format raw'
ssh libvirtactuator 'virsh vol-upload --pool default fedora_base /Fedora-Cloud-Base-28-1.1.x86_64.qcow2'

ssh libvirtactuator 'yum install genisoimage -y'

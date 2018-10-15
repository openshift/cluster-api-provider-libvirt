#!/bin/sh

export LIBVIRT_DEFAULT_URI=qemu+ssh://root@libvirtactuator/system

script_dir="$(cd $(dirname "${BASH_SOURCE[0]}") && pwd -P)"
# Upload cloud-init iso to bootstrap an instance
virsh vol-create-as default cloud-init.iso $(stat -Lc%s ${script_dir}/cloud-init.iso) --format raw
virsh vol-upload --pool default cloud-init.iso ${script_dir}/cloud-init.iso
scp ${script_dir}/guest.pem libvirtactuator:/root/.ssh/id_rsa
ssh libvirtactuator 'chmod 600 /root/.ssh/id_rsa'

# Use ipv4 since ipv6 timeouts
ssh libvirtactuator 'wget --inet4-only https://download.fedoraproject.org/pub/fedora/linux/releases/28/Cloud/x86_64/images/Fedora-Cloud-Base-28-1.1.x86_64.qcow2'
# Took from https://askubuntu.com/a/299578
ssh libvirtactuator 'virsh vol-create-as default fedora_base $(stat -Lc%s Fedora-Cloud-Base-28-1.1.x86_64.qcow2) --format raw'
ssh libvirtactuator 'virsh vol-upload --pool default fedora_base Fedora-Cloud-Base-28-1.1.x86_64.qcow2'

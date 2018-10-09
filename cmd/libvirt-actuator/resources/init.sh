#!/bin/sh

# Update the qemu-kvm to a version that has -fw_cfg flag (2.4 at least)
cat <<EOF |
[centos]
name=centos
baseurl=http://mirror.centos.org/centos/7/virt/x86_64/kvm-common/
gpgcheck=0
enabled=1
EOF
ssh libvirtactuator 'cat > /etc/yum.repos.d/centos.repo'

ssh libvirtactuator 'yum update qemu-kvm -y'

# Create default storage volume
export LIBVIRT_DEFAULT_URI=qemu+ssh://root@libvirtactuator/system

virsh pool-define /dev/stdin <<EOF
<pool type='dir'>
  <name>default</name>
  <target>
    <path>/var/lib/libvirt/images</path>
  </target>
</pool>
EOF

virsh pool-start default
virsh pool-autostart default

script_dir="$(cd $(dirname "${BASH_SOURCE[0]}") && pwd -P)"
# Upload cloud-init iso to bootstrap an instance
virsh vol-create-as default cloud-init.iso $(stat -Lc%s ${script_dir}/cloud-init.iso) --format raw
virsh vol-upload --pool default cloud-init.iso ${script_dir}/cloud-init.iso

# Use ipv4 since ipv6 timeouts
ssh libvirtactuator 'wget --inet4-only https://download.fedoraproject.org/pub/fedora/linux/releases/28/Cloud/x86_64/images/Fedora-Cloud-Base-28-1.1.x86_64.qcow2'
# Took from https://askubuntu.com/a/299578
ssh libvirtactuator 'virsh vol-create-as default fedora_base $(stat -Lc%s Fedora-Cloud-Base-28-1.1.x86_64.qcow2) --format raw'
ssh libvirtactuator 'virsh vol-upload --pool default fedora_base Fedora-Cloud-Base-28-1.1.x86_64.qcow2'

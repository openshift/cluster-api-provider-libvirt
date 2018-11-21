#/bin/bash

# Create default storage volume
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

# Start minikube
/usr/local/bin/minikube start --vm-driver kvm2 --kubernetes-version="v1.11.3" -v=5

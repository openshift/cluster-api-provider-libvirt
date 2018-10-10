#/bin/bash

yum install -y -d1 libvirt libvirt-daemon-kvm
usermod -aG libvirt root

# Enable ssh+qemu access mode
cat <<EOF > /etc/libvirt/libvirtd.conf
unix_sock_group = "libvirt"
unix_sock_rw_perms = "0770"
EOF

# Next lines are here if we would like to enable tcp+qemu conection mode
#cat <<EOF > /etc/libvirt/libvirtd.conf
#unix_sock_group = "libvirt"
#unix_sock_rw_perms = "0770"
#listen_tls = 0
#listen_tcp = 1
#auth_tcp="none"
#tcp_port = "16509"
#EOF
#echo 'LIBVIRTD_ARGS="--listen"' >> /etc/sysconfig/libvirtd
#iptables -I INPUT -p tcp --dport 16509 -j ACCEPT -m comment --comment "Allow insecure libvirt clients"

systemctl start libvirtd

# Install kubectl
cat <<EOF > /etc/yum.repos.d/kubernetes.repo
[kubernetes]
name=Kubernetes
baseurl=https://packages.cloud.google.com/yum/repos/kubernetes-el7-x86_64
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
EOF
yum -y install kubectl

# Install minikube
curl -Lo /tmp/minikube https://storage.googleapis.com/minikube/releases/v0.30.0/minikube-linux-amd64
chmod +x /tmp/minikube
cp /tmp/minikube /usr/local/bin/

# Install kvm2 driver
curl -Lo /tmp/docker-machine-driver-kvm2 https://storage.googleapis.com/minikube/releases/latest/docker-machine-driver-kvm2 
chmod +x /tmp/docker-machine-driver-kvm2
cp /tmp/docker-machine-driver-kvm2 /usr/local/bin/

# Start minikube
/usr/local/bin/minikube start --vm-driver kvm2

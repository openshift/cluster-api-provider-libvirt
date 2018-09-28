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

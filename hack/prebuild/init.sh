#/bin/bash

apt-get update
apt-get install -y libvirt-bin
usermod -aG libvirtd root

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

systemctl restart libvirtd

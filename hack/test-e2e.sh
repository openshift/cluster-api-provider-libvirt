#!/bin/bash
#shellcheck disable=SC2086

set -e

image="origin-libvirt-machine-controllers:latest"
nodelink_image="$(docker run registry.svc.ci.openshift.org/openshift/origin-release:v4.0 image machine-api-operator)"
script_dir="$(cd $(dirname "${BASH_SOURCE[0]}") && pwd -P)"

# Set packet instance connection parameters
RHOST="root@$(cat /tmp/packet_ip)"
SSH_OPTS="-o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes -o PasswordAuthentication=no -o StrictHostKeyChecking=no -i /tmp/packet_id_rsa"

# TODO remove after https://github.com/paulfantom/packet-image/pull/1
ssh ${SSH_OPTS} "${RHOST}" 'yum -y -d1 install docker && systemctl start docker'

# Import fedora-28 image into libvirt
ssh ${SSH_OPTS} "${RHOST}" 'virsh vol-create-as default fedora_base $(stat -Lc%s /Fedora-Cloud-Base-28-1.1.x86_64.qcow2) --format raw'
ssh ${SSH_OPTS} "${RHOST}" 'virsh vol-upload --pool default fedora_base /Fedora-Cloud-Base-28-1.1.x86_64.qcow2'

# Copy ssh keys to packet host
scp ${SSH_OPTS} ${script_dir}/../cmd/libvirt-actuator/resources/guest.pem "${RHOST}":/guest.pem
ssh ${SSH_OPTS} "${RHOST}" 'cp /guest.pem /root/.ssh/id_rsa'
scp ${SSH_OPTS} /tmp/packet_id_rsa "${RHOST}":/libvirt.pem
ssh ${SSH_OPTS} "${RHOST}" 'chmod 600 /libvirt.pem /guest.pem /root/.ssh/id_rsa'

# Copy built docker image to the instance
docker save "${image}" | bzip2 | ssh ${SSH_OPTS} "${RHOST}" "bunzip2 > /tmp/tempimage.bz2 && docker load -i /tmp/tempimage.bz2"
# Copy built docker image into the minikube guest
ssh ${SSH_OPTS} "${RHOST}" "docker save $image | bzip2 | ssh -o StrictHostKeyChecking=no -i ~/.minikube/machines/minikube/id_rsa docker@\$(minikube ip) 'bunzip2 > /tmp/tempimage.bz2 && sudo docker load -i /tmp/tempimage.bz2'"

# Copy and execute test binary on remote host
scp ${SSH_OPTS} bin/machines.test "${RHOST}:."
ssh ${SSH_OPTS} "${RHOST}" "./machines.test -logtostderr -v 5 -kubeconfig ~/.kube/config -ginkgo.v -machine-controller-image $image -machine-manager-image $image -nodelink-controller-image $nodelink_image -libvirt-uri 'qemu+ssh://${RHOST}/system?no_verify=1&keyfile=/libvirt.pem' -libvirt-pk /libvirt.pem -ssh-user fedora -ssh-key /guest.pem"

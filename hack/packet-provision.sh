#!/usr/bin/env bash
set +e

# Your Packet user account
if [ "$PACKET_AUTH_TOKEN" == "" ]; then
    echo "You need to set PACKET_AUTH_TOKEN variable first."
    echo "Make sure that your SSH key is also set in packet.net"
    exit 1
fi

export TF_VAR_environment_id=${ENVIRONMENT_ID:-$(uuidgen | cut -c1-8)}

cd ./prebuild
case ${1} in
  "install")
    ssh_path="$TF_VAR_ssh_key_path"
    if [ "$TF_VAR_ssh_key_path" == "" ]; then
        echo -e "\e[33mCreating temporary SSH file\e[0m"
        ssh-keygen -t rsa -b 4096 -C "temporary packet.net key" -P "" -f "/tmp/packet_id_rsa" -q
        ssh_path="/tmp/packet_id_rsa.pub"
    fi
    terraform init -input=false
    terraform plan -input=false -out=tfplan.out && terraform apply -input=false -auto-approve tfplan.out
    echo -e "\e[32m"
    echo -e "*** Your packet.net host is called ${TF_VAR_environment_id}"
    echo -e "*** You can also access it via SSH with key located in ${ssh_path}"
    echo -e "\e[0m"
    ;;
  "destroy")
    terraform destroy -input=false -auto-approve
    rm /tmp/packet_id_rsa* || :
    ;;
  *)
    echo "Use '$0 install' or '$0 destroy'."
    ;;
esac

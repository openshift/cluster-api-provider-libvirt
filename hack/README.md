# Packet instance provisioning

Our CI system uses packet.net to provision instances and tests code there. To accomplish this we are running 
`packet-provision.sh` script which is a thin wrapper on terraform code. This mechanism creates machines based on images
build by using [paulfantom/packet-image](https://github.com/paulfantom/packet-image) repository.

##### packet-provision.sh

Script is able to provision and deprovision one instance in packet.net. It also takes care of tagging that instance,
assigning unique name to it and creating temporary ssh key (Private key is located in `/tmp/packet_id_rsa` unless 
`TF_VAR_ssh_key_path` was previously set).
Script can be configured with following environment variables:
- `PACKET_AUTH_TOKEN` - packet.net API authentication token.
- `TF_VAR_packet_project_id` - packet.net project id.
- `TF_VAR_ssh_key_path` - path to SSH key (optional)

Script accepts on of two parameters:
- `install` - provision instance
- `destroy` - cleanup

##### Image reference

Currently we bake in reference to image which we want to use by using a commit id from master branch of
[paulfantom/packet-image](https://github.com/paulfantom/packet-image) repository. This is done in: 
https://github.com/openshift/cluster-api-provider-libvirt/blob/master/hack/prebuild/main.tf#L14

# Packet instance provisioning

Our CI system uses packet.net to provision instances and to test code there. To accomplish this we are running 
`packet-provision.sh` script which is a thin wrapper over terraform code. This mechanism creates machines based on images
built by using [paulfantom/packet-image](https://github.com/paulfantom/packet-image) repository.

##### packet-provision.sh

Script is able to provision and deprovision one instance in packet.net. It also takes care of tagging that instance,
assigning unique name to it and creating temporary ssh key (private key is located in `/tmp/packet_id_rsa` unless 
`TF_VAR_ssh_key_path` was previously set).
Script can be configured with following environment variables:
- `PACKET_AUTH_TOKEN` - packet.net API authentication token
- `TF_VAR_packet_project_id` - packet.net project id
- `TF_VAR_ssh_key_path` - path to SSH key (optional)

Script accepts one of two parameters:
- `install` - provision instance
- `destroy` - cleanup

Example usage:
```sh
PACKET_AUTH_TOKEN="..." ./hack/packet-provision.sh install
```

**Warning!**
Script doesn't clean after itself (doesn't run `destroy` by default) so invoker is responsible for cleaning resources
after provisioning them.

##### Image reference

Currently we hard code reference to the image which we want to use by using a commit id from master branch of
[paulfantom/packet-image](https://github.com/paulfantom/packet-image) repository. This is done by setting user_data
parameter in [`hack/prebuild/main.tf`](https://github.com/openshift/cluster-api-provider-libvirt/blob/master/hack/prebuild/main.tf): 
```yaml
user_data        = "#cloud-config\n#image_repo=https://github.com/paulfantom/packet-image.git\n#image_tag=3a3f1eb378f660b335a68b79f3af303380462652\nssh_pwauth: True"
```

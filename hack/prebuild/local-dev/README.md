# Setting up a local libvirt dev environment

## Pre-Requisites
Make sure you have the virsh binary installed: `sudo dnf install libvirt-client libvirt-devel`
### Configure default libvirt storage pool
Check to see if a default storage pool has been defined in Libvirt by running `virsh --connect qemu:///system pool-list`. If it does not exist, create it:
```
sudo virsh pool-define /dev/stdin <<EOF
<pool type='dir'>
  <name>default</name>
  <target>
    <path>/var/lib/libvirt/images</path>
  </target>
</pool>
EOF

sudo virsh pool-start default
sudo virsh pool-autostart default
```

### Install the terraform provider
Third party plugins are not automatically installed (See [terraform docs](https://www.terraform.io/docs/configuration/providers.html#third-party-plugins)), so we need to manually install it:
```
GOBIN=~/.terraform.d/plugins go get github.com/dmacvicar/terraform-provider-libvirt
```

### Cache terrafrom plugins
_(optional, but makes subsequent runs a bit faster)_
```
cat <<EOF > $HOME/.terraformrc
plugin_cache_dir = "$HOME/.terraform.d/plugin-cache"
EOF
```

## Run terraform
```
terraform init
terraform apply
```

To clean-up:
```
terraform destroy
```

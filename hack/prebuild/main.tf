resource "packet_ssh_key" "key" {
  name       = "unlikely_tf_ssh_key_name-${var.id}"
  public_key = "${file("${var.ssh_key_path}")}"
}

resource "packet_device" "libvirt" {
  hostname         = "libvirt-${var.id}"
  plan             = "baremetal_1"
  facility         = "sjc1"
  operating_system = "centos_7"
  billing_cycle    = "hourly"
  project_id       = "${var.packet_project_id}"
  tags             = "${list("${var.tag}")}"
  user_data        = "#cloud-config\n#image_repo=https://github.com/paulfantom/packet-image.git\n#image_tag=3a3f1eb378f660b335a68b79f3af303380462652\nssh_pwauth: True"

  provisioner "remote-exec" {
    script = "init.sh"

    connection = {
      type     = "ssh"
      user     = "root"
      password = "${self.root_password}"
      agent    = false
    }
  }

  depends_on = ["packet_ssh_key.key"]
}

output "ip" {
  value = "${packet_device.libvirt.access_public_ipv4}"
}

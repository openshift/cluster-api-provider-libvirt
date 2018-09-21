resource "packet_project" "libvirt_actuator" {
  name = "libvirt-actuator tests"
}

resource "packet_ssh_key" "key" {
  name       = "unlikely_tf_ssh_key_name"
  public_key = "${file("${var.ssh_key_path}")}"
}

resource "packet_device" "libvirt" {
  hostname         = "${var.environment_id}"
  plan             = "baremetal_0"
  facility         = "ewr1"
  operating_system = "centos_7"
  billing_cycle    = "hourly"
  project_id       = "${packet_project.libvirt_actuator.id}"
  user_data        = "#!/bin/bash\nsed -i 's/PasswordAuthentication.*$/PasswordAuthentication yes/g' /etc/ssh/sshd_config && systemctl restart sshd"
  provisioner "remote-exec" {
    script = "init.sh"
    connection = {
      type = "ssh"
      user = "root"
      password = "${self.root_password}"
      agent = false
    }
  }
}

output "ip" {
  value = "${packet_device.libvirt.access_public_ipv4}"
}


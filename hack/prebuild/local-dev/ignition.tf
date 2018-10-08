data "ignition_user" "core" {
  name                = "core"
  ssh_authorized_keys = ["${var.ssh_key}"]
}

data "ignition_config" "libvirt_config" {
  users = [
    "${data.ignition_user.core.id}",
  ]
}

resource "libvirt_ignition" "ignition" {
  count   = "${length(var.ignition_volumes)}"
  name    = "${element(keys(var.ignition_volumes[count.index]), 0)}"
  content = "${data.ignition_config.libvirt_config.rendered}"
}

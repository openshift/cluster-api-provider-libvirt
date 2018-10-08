provider "libvirt" {
  uri = "${var.libvirt_uri}"
}

resource "libvirt_network" "dev_net" {
  name   = "${var.libvirt_network_name}"
  mode   = "nat"
  bridge = "${var.libvirt_network_if}"
  domain = "${var.base_domain}"

  addresses = [
    "${var.libvirt_ip_range}",
  ]

  dns = {
    local_only = true

    forwarders = [{
      address = "${var.libvirt_resolver}"
    }]
  }

  autostart = true
}

resource "libvirt_volume" "rh_cos_base" {
  name   = "rh_cos_base"
  source = "${var.libvirt_base_image}"
}

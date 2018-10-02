variable "base_domain" {
  type        = "string"
  description = "The base DNS domain of the cluster."
  default     = "tt.testing"
}

variable "libvirt_uri" {
  type        = "string"
  description = "libvirt connection URI"
  default     = "qemu:///system"
}

variable "libvirt_network_name" {
  type        = "string"
  description = "Name of the libvirt network to create"
  default     = "dev"
}

variable "libvirt_network_if" {
  type        = "string"
  description = "The name of the bridge to use"
  default     = "tt0"
}

variable "libvirt_ip_range" {
  type        = "string"
  description = "IP range for the libvirt machines"
  default     = "192.168.124.0/24"
}

variable "libvirt_resolver" {
  type        = "string"
  description = "the upstream dns resolver"
  default     = "8.8.8.8"
}

# The path to fetch the base image from. This can be set to a remote location,
# or a local path. In order to fetch the image remotely, you will
# need to connect via VPN.
variable "libvirt_base_image" {
  type    = "string"
  default = "http://aos-ostree.rhev-ci-vms.eng.rdu2.redhat.com/rhcos/images/cloud/latest/rhcos-qemu.qcow2.gz"
}

# Ignition

variable "ignition_volumes" {
  type        = "list"
  description = "list of ignition volumes to be created"

  default = [
    "bootstrap.ign",
    "worker.ign",
    "master.ign",
  ]
}

variable "ssh_key" {
  type        = "string"
  description = "Contents of your SSH public key"
}

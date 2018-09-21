variable "ssh_key_path" {
  type    = "string"
  default = "/tmp/packet_id_rsa.pub"
}

variable "environment_id" {
  type    = "string"
  default = "testHypervisor"
}

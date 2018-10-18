variable "ssh_key_path" {
  type    = "string"
  default = "/tmp/packet_id_rsa.pub"
}

variable "id" {
  type    = "string"
  default = "randomid"
}

variable "packet_project_id" {
  type    = "string"
  default = ""
}

variable "tag" {
  type    = "string"
  default = "usermachine"
}

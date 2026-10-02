variable "vm_name" { type = string }
variable "node_name" { type = string }
variable "vm_id" { type = number }
variable "storage" { type = string }
variable "cpu_cores" { type = number }
variable "memory_mb" { type = number }
variable "disk_size" { type = number }
variable "ipv4_address" { type = string }
variable "gateway" { type = string }
variable "vm_user" { type = string }
variable "vm_ssh_public_key" { type = string }
variable "dns_servers" { type = list(string) }
variable "template_id" { type = number }
variable "user_password" {
  type      = string
  sensitive = true
}

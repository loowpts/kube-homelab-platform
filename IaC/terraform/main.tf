terraform {
  backend "http" {}
}

module "vm_cluster" {
  source   = "./modules/vm"
  for_each = var.virtual_machines

  # Параметры конкретной VM из virtual_machines
  vm_name      = each.key
  vm_id        = each.value.vm_id
  cpu_cores    = each.value.cpu_cores
  memory_mb    = each.value.memory_mb
  ipv4_address = each.value.ip_address
  disk_size    = each.value.disk_size

  # Общие для всех VM (homelab.auto.tfvars и секреты из TF_VAR_*)
  node_name         = var.proxmox_node
  storage           = var.storage
  gateway           = var.gateway
  dns_servers       = var.dns_servers
  vm_user           = var.vm_user
  user_password     = var.user_password
  vm_ssh_public_key = var.vm_ssh_public_key
  template_id       = var.template_id
}


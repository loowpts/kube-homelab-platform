variable "proxmox_url" {
  description = "Proxmox API URL (узнать IP: ip a на Proxmox хосте)"
  type        = string
  default     = "https://192.168.0.143:8006"
}

variable "proxmox_token" {
  description = "API токен: pveum user token add terraform@pve terraform-token --privsep=0"
  type        = string
  sensitive   = true

  validation {
    condition     = length(trimspace(var.proxmox_token)) > 0
    error_message = "proxmox_token пустой — задайте CI-переменную PROXMOX_TOKEN (или terraform.tfvars локально)"
  }
}

variable "proxmox_ssh_password" {
  description = "Пароль root на Proxmox хосте (нужен провайдеру bpg/proxmox для SSH)"
  type        = string
  sensitive   = true

  validation {
    condition     = length(trimspace(var.proxmox_ssh_password)) > 0
    error_message = "proxmox_ssh_password пустой — задайте CI-переменную PROXMOX_SSH_PASSWORD"
  }
}

variable "proxmox_node" {
  description = "Имя Proxmox ноды (узнать: hostname на Proxmox хосте)"
  type        = string
}

variable "template_id" {
  description = "VM ID для Ubuntu шаблона (9000+ принято резервировать под шаблоны)"
  type        = number
}

variable "user_password" {
  description = "Пароль пользователя VM (cloud-init)"
  type        = string
  sensitive   = true

  validation {
    condition     = length(trimspace(var.user_password)) > 0
    error_message = "user_password пустой — задайте CI-переменную VM_USER_PASSWORD"
  }
}
variable "storage" {
  description = "Хранилище для дисков VM (узнать: pvesm status на Proxmox)"
  type        = string
}

variable "vm_ssh_public_key" {
  description = "Публичный SSH ключ (cat ~/.ssh/id_ed25519.pub)"
  type        = string

  validation {
    condition     = length(trimspace(var.vm_ssh_public_key)) > 0
    error_message = "vm_ssh_public_key пустой — задайте CI-переменную VM_SSH_PUBLIC_KEY; иначе план удалит SSH-ключ у всех VM"
  }
}

variable "gateway" {
  description = "IP шлюза твоей локальной сети"
  type        = string
}

variable "vm_user" {
  description = "Имя пользователя во всех VM (cloud-init создаёт его автоматически)"
  type        = string
}

variable "dns_servers" {
  description = "DNS серверы для всех VM"
  type        = list(string)
}

variable "virtual_machines" {
  type = map(object({
    vm_id      = number
    cpu_cores  = number
    memory_mb  = number
    ip_address = string
    disk_size  = number
  }))
  description = "Карта виртуальных машин для создания через модуль"
}

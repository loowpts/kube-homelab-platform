terraform {
  required_version = ">= 1.5"
  required_providers {
    proxmox = {
      source  = "bpg/proxmox"
      version = ">= 0.114.0, < 1.0.0"
    }
  }
}

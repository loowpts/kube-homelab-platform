resource "proxmox_virtual_environment_vm" "this" {
  name      = var.vm_name
  node_name = var.node_name
  vm_id     = var.vm_id
  started   = true

  # Никогда не перезагружать VM автоматически: apply меняет все VM параллельно и
  # перезагрузил бы, например, все три мастера сразу (потеря кворума etcd).
  # Изменения, требующие перезагрузки (память, CPU), становятся pending —
  # применить по одной VM: qm reboot <vmid> на хосте Proxmox
  # (перезагрузка изнутри VM pending-изменения не применяет).
  reboot_after_update = false

  # Автоматически выключать VM перед удалением
  stop_on_destroy = true

  # Полностью удалять диски из хранилища при уничтожении
  purge_on_destroy = true

  clone {
    vm_id = var.template_id
    full  = true
  }

  agent {
    enabled = true
  }

  cpu {
    cores = var.cpu_cores
    type  = "host"
  }

  memory {
    dedicated = var.memory_mb
  }

  disk {
    datastore_id = var.storage
    interface    = "scsi0"
    size         = var.disk_size
    discard      = "on"
    iothread     = false
  }

  network_device {
    bridge = "vmbr0"
  }

  initialization {
    datastore_id = var.storage

    ip_config {
      ipv4 {
        address = var.ipv4_address
        gateway = var.gateway
      }
    }

    user_account {
      username = var.vm_user
      keys     = [var.vm_ssh_public_key]
      password = var.user_password
    }

    dns {
      servers = var.dns_servers
      domain  = "local"
    }
  }
}

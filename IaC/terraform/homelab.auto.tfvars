# Несекретные параметры стенда. Коммитится в git и подхватывается Terraform автоматически
# (*.auto.tfvars). Секреты — в terraform.tfvars (локально) или TF_VAR_* (CI).

# --- Общие параметры стенда ---
proxmox_node = "node"
storage      = "local-lvm"
gateway      = "192.168.0.1"
dns_servers  = ["192.168.0.1", "8.8.8.8"]

# --- Доступ к ВМ (ключ и пароль — секреты, см. terraform.tfvars.example) ---
vm_user = "deploy"
# --- ID шаблона в Proxmox ---
template_id = 999

# --- Карта всех виртуальных машин стенда ---
virtual_machines = {
  "master1" = {
    vm_id      = 2000
    cpu_cores  = 2
    memory_mb  = 5120
    ip_address = "192.168.0.100/24"
    disk_size  = 35
  }

  "master2" = {
    vm_id      = 2001
    cpu_cores  = 2
    memory_mb  = 5120
    ip_address = "192.168.0.101/24"
    disk_size  = 35
  }

  "master3" = {
    vm_id      = 2002
    cpu_cores  = 2
    memory_mb  = 5120
    ip_address = "192.168.0.102/24"
    disk_size  = 35
  }

  "worker1" = {
    vm_id      = 3000
    cpu_cores  = 2
    memory_mb  = 4096
    ip_address = "192.168.0.110/24"
    disk_size  = 35

  }

  "worker2" = {
    vm_id      = 3001
    cpu_cores  = 2
    memory_mb  = 4096
    ip_address = "192.168.0.111/24"
    disk_size  = 35
  }

  "infra" = {
    vm_id      = 3002
    cpu_cores  = 4
    memory_mb  = 8192
    ip_address = "192.168.0.112/24"
    disk_size  = 50
  }

  "lb1" = {
    vm_id      = 200
    cpu_cores  = 2
    memory_mb  = 1024
    ip_address = "192.168.0.10/24"
    disk_size  = 35
  }

  "lb2" = {
    vm_id      = 300
    cpu_cores  = 2
    memory_mb  = 1024
    ip_address = "192.168.0.11/24"
    disk_size  = 35
  }

}

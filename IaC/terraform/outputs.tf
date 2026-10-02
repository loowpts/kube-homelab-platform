output "node_ips" {
  value = {
    for k, node in module.vm_cluster : "node-${k}" => node.ipv4_address
  }
}

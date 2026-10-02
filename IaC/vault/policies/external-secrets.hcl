# External Secrets Operator: только чтение KV v2 в secret/
path "secret/data/*" {
  capabilities = ["read"]
}

path "secret/metadata/*" {
  capabilities = ["read", "list"]
}

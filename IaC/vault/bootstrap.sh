#!/usr/bin/env bash
# Инициализация и настройка Vault для External Secrets Operator. Идемпотентен.
#
#   ./bootstrap.sh                     init (только при первом запуске) + unseal + настройка
#   ./bootstrap.sh --restore-from-k8s  то же + сначала вернуть в Vault секреты из Secret'ов кластера
#                                      (todo-dev/todo-prod: postgres, redis, registry; monitoring: grafana)
# Секреты приложения secret/{dev,prod}/{postgres,redis} создаются автоматически, если их нет.
#
# При первом запуске root token и unseal key сохраняются в $VAULT_INIT_FILE
# (по умолчанию ~/.config/homelab/vault-init.json, права 600) — перенесите их
# в менеджер паролей. Для повторных запусков root token берётся оттуда же,
# из переменной VAULT_TOKEN или спрашивается интерактивно.
set -euo pipefail

NS=vault
POD=vault-0
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
INIT_FILE="${VAULT_INIT_FILE:-$HOME/.config/homelab/vault-init.json}"
RESTORE=false
[[ "${1:-}" == "--restore-from-k8s" ]] && RESTORE=true

vault_cmd() {
  kubectl -n "$NS" exec -i "$POD" -c vault -- env VAULT_TOKEN="${VAULT_TOKEN:-}" vault "$@"
}

# Поле из `vault status -format=json` (exit code 2 при sealed — это не ошибка)
status_field() {
  local out
  out=$(vault_cmd status -format=json 2>/dev/null || true)
  python3 -c "import sys,json;print(str(json.loads(sys.argv[1])['$1']).lower())" "$out" 2>/dev/null || true
}

# Secret кластера -> JSON {ключ: значение} для `vault kv put <path> -`
# k8s_secret_json <namespace> <secret> [ключи через пробел — по умолчанию все]
k8s_secret_json() {
  kubectl -n "$1" get secret "$2" -o json | python3 -c '
import sys, json, base64
d = json.load(sys.stdin)["data"]
keys = sys.argv[1:] or list(d)
print(json.dumps({k: base64.b64decode(d[k]).decode() for k in keys}))' "${@:3}"
}

echo "==> Ждём под $POD"
kubectl -n "$NS" wait --for=jsonpath='{.status.phase}'=Running "pod/$POD" --timeout=300s >/dev/null

if [[ "$(status_field initialized)" != "true" ]]; then
  echo "==> Vault не инициализирован: vault operator init"
  mkdir -p "$(dirname "$INIT_FILE")"
  (umask 077 && vault_cmd operator init -key-shares=1 -key-threshold=1 -format=json > "$INIT_FILE")
  unseal_key=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['unseal_keys_b64'][0])" "$INIT_FILE")
  kubectl -n "$NS" create secret generic vault-unseal-key --from-literal=key="$unseal_key" \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  echo "    root token и unseal key сохранены в $INIT_FILE — перенесите их в менеджер паролей"
fi

if [[ "$(status_field sealed)" == "true" ]]; then
  echo "==> Unseal"
  unseal_key=$(kubectl -n "$NS" get secret vault-unseal-key -o jsonpath='{.data.key}' | base64 -d)
  vault_cmd operator unseal "$unseal_key" >/dev/null
fi

if [[ -z "${VAULT_TOKEN:-}" ]]; then
  if [[ -f "$INIT_FILE" ]]; then
    VAULT_TOKEN=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['root_token'])" "$INIT_FILE")
  else
    read -rsp "Vault root token: " VAULT_TOKEN
    echo
  fi
fi
export VAULT_TOKEN

echo "==> KV v2 в secret/"
mounts=$(vault_cmd secrets list -format=json)
grep -q '"secret/"' <<<"$mounts" || vault_cmd secrets enable -path=secret kv-v2 >/dev/null

echo "==> Kubernetes auth"
auths=$(vault_cmd auth list -format=json)
grep -q '"kubernetes/"' <<<"$auths" || vault_cmd auth enable kubernetes >/dev/null
# Внутри кластера Vault сам берёт CA и свой ServiceAccount-токен для TokenReview
vault_cmd write auth/kubernetes/config kubernetes_host="https://kubernetes.default.svc:443" >/dev/null

echo "==> Политика и роль для External Secrets (ClusterSecretStore в IaC/argocd/manifests/external-secrets)"
vault_cmd policy write external-secrets - < "$SCRIPT_DIR/policies/external-secrets.hcl" >/dev/null
vault_cmd write auth/kubernetes/role/external-secrets-role \
  bound_service_account_names=external-secrets \
  bound_service_account_namespaces=external-secrets \
  policies=external-secrets \
  ttl=1h >/dev/null

# Восстановление (если Vault потерял данные): значения берутся из Secret'ов, которые
# ESO создал в кластере раньше. Идёт до генерации — иначе приложения получили бы
# новые пароли, которых не знает их БД.
if [[ "$RESTORE" == true ]]; then
  echo "==> Секреты из кластера -> Vault"
  for env in dev prod; do
    ns="todo-${env}"
    if kubectl -n "$ns" get secret postgres >/dev/null 2>&1; then
      k8s_secret_json "$ns" postgres user password db | vault_cmd kv put "secret/${env}/postgres" - >/dev/null
      k8s_secret_json "$ns" redis password | vault_cmd kv put "secret/${env}/redis" - >/dev/null
      echo "    secret/${env}/{postgres,redis} — восстановлены из ${ns}"
    fi
  done

  for ns in todo-prod todo-dev; do
    if kubectl -n "$ns" get secret gitlab-registry-secret >/dev/null 2>&1; then
      kubectl -n "$ns" get secret gitlab-registry-secret -o json | python3 -c '
import sys, json, base64
cfg = json.loads(base64.b64decode(json.load(sys.stdin)["data"][".dockerconfigjson"]))
auth = next(iter(cfg["auths"].values()))  # единственный registry в dockerconfig
print(json.dumps({"username": auth["username"], "token": auth["password"]}))' \
        | vault_cmd kv put secret/gitlab-registry - >/dev/null
      echo "    secret/gitlab-registry — восстановлен из ${ns}"
      break
    fi
  done

  if kubectl -n monitoring get secret grafana-admin >/dev/null 2>&1; then
    k8s_secret_json monitoring grafana-admin admin-user admin-password | vault_cmd kv put secret/grafana - >/dev/null
    echo "    secret/grafana — восстановлен"
  fi
fi

echo "==> Секреты приложения todo по окружениям (создаются, только если их ещё нет)"
gen_password() { python3 -c 'import secrets; print(secrets.token_hex(24))'; }
for env in dev prod; do
  if vault_cmd kv get -field=user "secret/${env}/postgres" >/dev/null 2>&1; then
    echo "    secret/${env}/postgres — уже есть"
  else
    printf '{"user":"todo","db":"todo","password":"%s"}' "$(gen_password)" \
      | vault_cmd kv put "secret/${env}/postgres" - >/dev/null
    echo "    secret/${env}/postgres — создан"
  fi
  if vault_cmd kv get -field=password "secret/${env}/redis" >/dev/null 2>&1; then
    echo "    secret/${env}/redis — уже есть"
  else
    printf '{"password":"%s"}' "$(gen_password)" | vault_cmd kv put "secret/${env}/redis" - >/dev/null
    echo "    secret/${env}/redis — создан"
  fi
done

echo "==> Пересинхронизация ExternalSecret"
kubectl annotate externalsecret -A --all force-sync="$(date +%s)" --overwrite >/dev/null
kubectl wait --for=condition=Ready clustersecretstore/vault-backend --timeout=60s
kubectl wait --for=condition=Ready externalsecret --all --all-namespaces --timeout=120s

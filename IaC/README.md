# IaC

Инфраструктура стенда: Proxmox VM (Terraform) → подготовка хостов и LB (Ansible) → кластер (kubespray) → платформа (Ansible).

```
IaC/
  terraform/             VM в Proxmox; state — в GitLab этого проекта (Operate → Terraform states)
    homelab.auto.tfvars  несекретные параметры стенда (в git)
    terraform.tfvars     локальные секреты (НЕ в git, шаблон — terraform.tfvars.example)
  ansible/               base/node/loadbalancer (site.yml) и platform.yml
  inventory/mycluster/   инвентарь и group_vars для kubespray
  kubespray/             git submodule kubernetes-sigs/kubespray (v2.32.0), сторонний проект — не редактируется
  argocd/root.yaml       app of apps: ArgoCD применяет всё из argocd/apps
  argocd/apps/           компоненты платформы (Vault, ...) как ArgoCD Application
  vault/bootstrap.sh     init / unseal / настройка Vault для External Secrets
  ci/                    шаблоны джоб; подключаются из корневого .gitlab-ci.yml
```

## Первый клон

```bash
git clone --recurse-submodules <repo>
# или в уже склонированном репо:
git submodule update --init
```

`IaC/kubespray` — submodule официального репозитория
[kubernetes-sigs/kubespray](https://github.com/kubernetes-sigs/kubespray) на теге `v2.32.0`
(Apache-2.0). Код kubespray в этом репозитории не хранится и не меняется; свои здесь только
inventory и `group_vars` в `IaC/inventory/mycluster`, сделанные на основе его `inventory/sample`.

## CI/CD Variables проекта homelab

Пайплайн ожидает переменные (Settings → CI/CD → Variables, masked + protected):

| Переменная | Назначение |
|---|---|
| `PROXMOX_TOKEN` | API-токен Proxmox (`user@realm!name=value`) |
| `PROXMOX_SSH_PASSWORD` | пароль SSH для провайдера bpg/proxmox |
| `VM_USER_PASSWORD` | пароль пользователя VM (cloud-init) |
| `VM_SSH_PUBLIC_KEY` | публичный ключ для VM |
| `PROXMOX_URL` | необязательно, по умолчанию `https://192.168.0.143:8006` |
| `SSH_KEY` | приватный ключ пользователя `deploy` для Ansible |
| `KEEPALIVED_AUTH_PASS` | пароль VRRP между lb1 и lb2 (первые 8 символов); локально — `export KEEPALIVED_AUTH_PASS=...` перед `loadbalancer.yml` |

## Terraform state

State хранится в GitLab этого проекта: Operate → Terraform states → `default`.
CI ходит в него встроенным `CI_JOB_TOKEN`. `terraform_plan` останавливается, если state
пустой: так ошибка в адресе или токене не превращается в «создать все VM заново». Для подъёма
стенда с нуля запустите пайплайн вручную с переменной `TF_ALLOW_EMPTY_STATE=true`.

Локально (личный токен со scope `api`):

```bash
cd IaC/terraform
read -s "T?GitLab token: "; echo
S=http://192.168.0.21/api/v4/projects/loowpts%2Fhomelab/terraform/state/default
terraform init -reconfigure \
  -backend-config="address=$S" -backend-config="lock_address=$S/lock" -backend-config="unlock_address=$S/lock" \
  -backend-config="username=loowpts" -backend-config="password=$T" \
  -backend-config="lock_method=POST" -backend-config="unlock_method=DELETE"
terraform plan
```

## Провайдер Terraform

CI инициализирует Terraform из локального зеркала (`init -plugin-dir=IaC/terraform/providers`),
без обращения к registry. Бинарник провайдера в репозиторий не входит; зеркало создаётся один раз:

```bash
cd IaC/terraform
terraform providers mirror -platform=linux_amd64 providers
```

## Пайплайн

Корневой `.gitlab-ci.yml` по изменённым путям запускает независимые дочерние пайплайны.
Ручной запуск (Build → Pipelines → Run pipeline) создаёт все три.

| Дочерний пайплайн | Когда запускается | Джобы |
|---|---|---|
| `terraform` | изменения в `IaC/terraform`, его CI | `lint_format`, `tflint` → `terraform_plan` (план в артефакт) → в `main` вручную `terraform_apply` (сохранённый план) / `terraform_destroy` |
| `ansible` | изменения в `IaC/ansible`, `IaC/inventory`, `IaC/kubespray`, его CI | `ansible_syntax` → в `main`: `ansible_ping` → `ansible_provision`; вручную `kubespray` → `platform` |
| `app` | изменения в `app/` | см. `app/README.md` |

Если меняются и Terraform, и Ansible (например, новая VM), пайплайны идут параллельно:
сначала нажмите `terraform_apply`, затем перезапустите `ansible_provision`.

## Локальный запуск kubespray

kubespray проверяет версию Ansible, поэтому запускать его нужно из venv с его `requirements.txt`,
а не системным Ansible:

```bash
cd IaC/kubespray
python3 -m venv venv && source venv/bin/activate
pip install -r requirements.txt
ansible-playbook -i ../inventory/mycluster/inventory.ini --user=deploy --become cluster.yml
```

## Обновление пакетов на нодах

`base.yml` по умолчанию не делает `apt dist-upgrade`. Обновлять по одной ноде:

```bash
cd IaC/ansible
ansible-playbook playbooks/base.yml -e apt_upgrade=true --limit node-worker1
```

Для нод кластера сначала `kubectl drain`, потом `kubectl uncordon`.

## ArgoCD: app of apps

Ansible (`platform.yml`) ставит только сам ArgoCD. Всё остальное описано в `IaC/argocd/apps/`,
ArgoCD синхронизирует это из ветки `main`. Порядок при подъёме с нуля задают sync-wave:

| Волна | Application | Что это |
|---|---|---|
| -3 | `longhorn`, `metrics-server` | хранилище (StorageClass `longhorn`), метрики для HPA |
| -2 | `cert-manager`, `external-secrets`, `ingress-nginx` | сертификаты, секреты из Vault, ingress |
| -1 | `vault`, `monitoring` | Vault (данные на Longhorn), Prometheus/Grafana/Alertmanager |
| 0 | `todo` (ApplicationSet → `todo-dev`, `todo-prod`) | приложение (`charts/todo`) |

Версии чартов зафиксированы (`targetRevision`). Обновление компонента — смена версии
в его файле и merge в `main`.

Компонентами управляет только ArgoCD: `helm install/upgrade/uninstall` вручную не используются,
ArgoCD рендерит чарты сам и применяет результат.

Подъём платформы с нуля: `ansible-playbook playbooks/platform.yml` (ArgoCD) → доступ
к репо и `kubectl apply -f IaC/argocd/root.yaml` (ниже) → после волны -1
`IaC/vault/bootstrap.sh` (init Vault + секреты; значения — из менеджера паролей).

Один раз: дать ArgoCD доступ к репозиторию (GitLab → homelab → Settings → Repository →
Deploy tokens, scope `read_repository`) и применить root:

```bash
read -rp "Deploy token username: " U; read -rsp "Deploy token: " P; echo
kubectl -n argocd create secret generic repo-homelab \
  --from-literal=type=git \
  --from-literal=url=http://192.168.0.21/loowpts/homelab.git \
  --from-literal=username="$U" --from-literal=password="$P"
kubectl -n argocd label secret repo-homelab argocd.argoproj.io/secret-type=repository
unset U P

kubectl apply -f IaC/argocd/root.yaml
```

## Размещение: infra и workers

У нод роли — метки `node-role.kubernetes.io/<роль>` (колонка ROLES в `kubectl get nodes`).
Ставит их роль `01-node-roles` в `platform.yml`, состав — группы `infra` и `workers` в
`IaC/ansible/inventory/hosts.ini`.

| Нода | Метка | Taint | Что работает |
|---|---|---|---|
| master1..3 | `control-plane` | `control-plane:NoSchedule` (kubespray) | control plane, etcd |
| infra | `infra` | `node-role.kubernetes.io/infra:NoSchedule` | платформа: ingress-nginx, мониторинг, Vault, ArgoCD, cert-manager, ESO, metrics-server, UI Longhorn |
| worker1, worker2 | `worker` | — | приложение: backend, postgres, redis, бэкап (`placement` в `charts/todo/values.yaml`) |
| все | — | — | DaemonSet'ы: calico, kube-proxy, node-exporter, Longhorn |

- Платформа: `nodeSelector` на infra + toleration (values в `IaC/argocd/apps/*.yaml`, для ArgoCD — `roles/02-argocd`).
- Приложение: `nodeSelector` на worker; toleration нет, поэтому на infra оно не попадёт, даже если
  селектор убрать. Реплики backend разнесены по воркерам (`topologySpreadConstraints`, `DoNotSchedule`).
- Новый компонент платформы — те же `nodeSelector` и `tolerations`; новое приложение — на worker.
- Longhorn работает на всех нодах (тома есть и на infra): toleration в `global` и
  `defaultSettings.taintToleration` (`IaC/argocd/apps/longhorn.yaml`).

**Taint ставится только после того, как Longhorn применил toleration** — иначе instance-manager
на infra после рестарта не встанет, и тома Prometheus и Grafana станут недоступны. Longhorn
применяет её только при отключённых томах, поэтому на работающем кластере это окно обслуживания
(простой ~5–10 минут): остановить автосинхронизацию ArgoCD и потребителей томов (postgres, Vault,
Prometheus, Grafana), дождаться `status.applied` у setting `taint-toleration`, поставить taint
(`platform.yml`) и вернуть всё обратно.

Роль `01-node-roles` сама taint не ставит, пока toleration в Longhorn не применена, — сообщает
об этом и пропускает. При подъёме с нуля окно не нужно: Longhorn сразу ставится с toleration.

## Доступ: Ingress

Все веб-сервисы — через ingress-nginx по имени хоста. Имена `*.192.168.0.15.nip.io`
публичный DNS nip.io резолвит в VIP, свой DNS и `/etc/hosts` не нужны.
Цепочка: `<VIP>:80/443` (HAProxy, tcp) → NodePort 31082/30214 → ingress-nginx → Service.

| Сервис | Адрес | Напрямую (в обход ingress) |
|---|---|---|
| todo prod | http://todo.192.168.0.15.nip.io, http://192.168.0.15 | — |
| todo dev | http://todo-dev.192.168.0.15.nip.io | http://192.168.0.15:8081 |
| Grafana | http://grafana.192.168.0.15.nip.io | :3000 |
| Prometheus | http://prometheus.192.168.0.15.nip.io | :9090 |
| Alertmanager | http://alertmanager.192.168.0.15.nip.io | — |
| Vault | http://vault.192.168.0.15.nip.io | :8200 |
| ArgoCD | https://argocd.192.168.0.15.nip.io | https://192.168.0.15:8443 |

Каждый адрес работает и по `https://`: сертификаты выпускает cert-manager от собственного CA
(ClusterIssuer `homelab-ca`, `IaC/argocd/manifests/cert-manager`). Редиректа на https нет,
чтобы http работал без импорта CA. Чтобы браузер доверял https — один раз импортировать корень:

```bash
kubectl -n cert-manager get secret homelab-ca -o jsonpath='{.data.ca\.crt}' | base64 -d > homelab-ca.crt
# macOS:
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain homelab-ca.crt
```

Новый сервис: Ingress с `ingressClassName: nginx`, host `<имя>.192.168.0.15.nip.io`,
для https — аннотация `cert-manager.io/cluster-issuer: homelab-ca` и `tls` с `secretName`.
HAProxy при этом не меняется.

Если резолвер роутера отбрасывает ответы с локальными адресами (защита от DNS rebinding),
nip.io не сработает — тогда имена в `/etc/hosts` на `192.168.0.15` или в DNS роутера.

## Vault

Standalone, данные на PVC Longhorn, после рестарта распечатывается автоматически
(сайдкар `auto-unseal`, ключ в Secret `vault/vault-unseal-key`). Описание —
`IaC/argocd/apps/vault.yaml`, чарт берётся из GitHub (Helm-репозиторий HashiCorp из РФ отвечает 403).

Первый запуск и восстановление настроек (auth kubernetes, политика, роль для ESO):

```bash
IaC/vault/bootstrap.sh                     # init при первом запуске + настройка
IaC/vault/bootstrap.sh --restore-from-k8s  # + вернуть в Vault секреты из Secret'ов кластера
```

Root token и unseal key после init — в `~/.config/homelab/vault-init.json`
(права 600); перенесите их в менеджер паролей.

## Бэкапы

Хранилище — NFS на хосте Proxmox (`192.168.0.143:/srv/backups`), вне кластера:

| Что | Как | Расписание | Хранится |
|---|---|---|---|
| Все тома Longhorn (Vault, Postgres, Grafana, Prometheus, новые PVC) | RecurringJob `daily-backup` → `/srv/backups/longhorn` | 17:00 UTC (20:00 МСК) | 3 последних (инкрементально) |
| БД приложения (prod) | CronJob `todo-prod/postgres-backup` (`pg_dump -Fc`) → `/srv/backups/postgres` | 17:30 UTC (20:30 МСК) | 14 дней |

Расписание — в часы, когда стенд включён: хост Proxmox на ночь выключается. У CronJob
`startingDeadlineSeconds: 3600` — пропущенный запуск не догоняется утром, пока кластер поднимается.

Свежесть бэкапов контролируют алерты: `LonghornVolumeBackupStale` (метрика `longhorn_volume_last_backup_at`,
[manifests/monitoring/longhorn.yaml](argocd/manifests/monitoring/longhorn.yaml)) и `PostgresBackupStale`
(`kube_cronjob_status_last_successful_time`, правила чарта). Порог — 26 часов.

Бэкапы переживают потерю кластера, но не потерю самого хоста Proxmox. Для этого следующий
шаг — S3 на другом железе (в Longhorn меняется только `backupTarget`).

Настройка (однократно, по порядку):

```bash
cd IaC/ansible
ansible-playbook playbooks/backup-nfs.yml   # NFS-сервер на Proxmox (root по SSH-ключу)
ansible-playbook playbooks/platform.yml     # nfs-common на нодах
# затем merge в main — ArgoCD применит backup target, расписание и CronJob
```

Проверка:

```bash
kubectl -n longhorn-system get backuptargets.longhorn.io   # default: AVAILABLE true
kubectl -n todo-prod create job --from=cronjob/postgres-backup pg-backup-manual
kubectl -n todo-prod logs job/pg-backup-manual              # backup ok: /backups/todo-prod-todo-....dump
ssh root@192.168.0.143 ls -lh /srv/backups/postgres /srv/backups/longhorn
```

Восстановление Postgres из дампа (сначала в отдельную БД, сверить, потом при необходимости в рабочую):

```bash
kubectl -n todo-prod run pg-restore --rm -it --image=postgres:16-alpine --restart=Never \
  --overrides='{"spec":{"volumes":[{"name":"b","nfs":{"server":"192.168.0.143","path":"/srv/backups/postgres"}}],
  "containers":[{"name":"pg-restore","image":"postgres:16-alpine","stdin":true,"tty":true,"command":["sh"],
  "volumeMounts":[{"name":"b","mountPath":"/backups"}]}]}}'
# внутри пода (пароль спросит; он в Vault, secret/prod/postgres):
createdb -h postgres -U todo restore_check
pg_restore -h postgres -U todo -d restore_check --no-owner /backups/<файл>.dump
psql -h postgres -U todo -d restore_check -c 'select count(*) from tasks'
```

Восстановление тома целиком (например Vault): Longhorn UI → Backup → выбрать бэкап → Restore
в новый том → подменить PVC. UI доступен через
`kubectl -n longhorn-system port-forward svc/longhorn-frontend 8080:80`.

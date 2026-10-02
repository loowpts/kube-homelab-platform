# todo-app

Go-сервис списка задач: REST API и серверный HTML-интерфейс поверх Postgres и Redis.
Рабочая нагрузка платформы: на нём проверяются пробы, rolling update, HPA, секреты из Vault,
метрики, алерты и бэкапы.

## Стек

- Go, [chi](https://github.com/go-chi/chi) — роутинг и middleware
- Postgres (pgx) — хранилище; миграции [golang-migrate](https://github.com/golang-migrate/migrate),
  SQL встроен в бинарник через `go:embed` и применяется отдельным запуском `./server -migrate-only`
  (в compose — сервис `migrate`, в Kubernetes — initContainer)
- Redis — кэш списка задач и общий для всех реплик rate limit на `/api/*`
- `html/template` — серверный рендеринг, без JS-сборки
- Prometheus client — RED-метрики на отдельном порту

## Структура

```
cmd/server/            точка входа: конфиг, подключения, роутинг, graceful shutdown
internal/config/       настройки из переменных окружения
internal/db/           пул соединений Postgres и запуск миграций
internal/cache/        клиент Redis
internal/models/       модель Task
internal/repository/   SQL-запросы (CRUD над tasks)
internal/service/      бизнес-логика: репозиторий + кэш списка в Redis
internal/handler/      HTTP: health, JSON API, HTML-страницы
internal/middleware/   метрики, real IP от ingress, rate limit
internal/integration/  интеграционные тесты (build tag integration)
migrations/            SQL-миграции up/down
web/                   шаблоны и CSS
Dockerfile             multi-stage сборка, non-root
docker-compose.yml     app + postgres + redis для локального запуска
```

Слои разделены: `handler` не знает про SQL, `repository` — про HTTP и Redis, а JSON API
и HTML-страницы работают через один и тот же `service`.

## API

### Health

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/healthz` | liveness: процесс жив и отвечает; зависимости не проверяет |
| GET | `/readyz` | readiness: `503`, если недоступен Postgres; состояние Redis — в теле ответа |

```
GET /readyz → 200 {"postgres":"ok","redis":"ok"}
            → 200 {"postgres":"ok","redis":"unavailable"}
            → 503 {"postgres":"unavailable","redis":"ok"}
```

Liveness не зависит от БД и Redis: временный сбой зависимости не должен перезапускать поды
по кругу. Redis — мягкая зависимость: без него кэш промахивается, а rate limit пропускает
запросы (fail open, с предупреждением в логе). Если бы readiness падала из-за Redis, его сбой
вывел бы из Service все реплики сразу и превратился в полный отказ.

### JSON API

Под rate limit: `RATE_LIMIT_RPM` запросов в минуту на IP (по умолчанию 120), при превышении —
`429` с `Retry-After`.

| Метод | Путь | Тело | Ответ |
|---|---|---|---|
| GET | `/api/tasks` | — | `200` список (из кэша Redis) |
| POST | `/api/tasks` | `{"title", "description"}` | `201` задача |
| GET | `/api/tasks/{id}` | — | `200` / `404` |
| PUT | `/api/tasks/{id}` | `{"title", "description", "done"}` | `200` / `404` |
| DELETE | `/api/tasks/{id}` | — | `204` / `404` |

```bash
curl -s -X POST http://localhost:8080/api/tasks \
  -H 'Content-Type: application/json' \
  -d '{"title":"Обновить кластер","description":"kubespray upgrade-cluster.yml"}'
```

### HTML

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/` | список задач |
| GET | `/tasks/new` | форма создания |
| POST | `/tasks` | создать |
| GET | `/tasks/{id}/edit` | форма редактирования |
| POST | `/tasks/{id}` | обновить |
| POST | `/tasks/{id}/delete` | удалить |

### Метрики

`/metrics` на `METRICS_PORT` — отдельный порт, через Ingress не публикуется; Prometheus
забирает его по ServiceMonitor. `http_requests_total` и `http_request_duration_seconds`
размечены шаблоном маршрута (`/api/tasks/{id}`), а не путём, — кардинальность ограничена.

## Конфигурация

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `HTTP_PORT` | `8080` | порт приложения |
| `METRICS_PORT` | `9090` | порт `/metrics` |
| `POSTGRES_DSN` | `postgres://postgres:postgres@localhost:5432/todoapp?sslmode=disable` | подключение к Postgres |
| `REDIS_ADDR` | `localhost:6379` | адрес Redis |
| `REDIS_PASSWORD` | — | пароль Redis |
| `REDIS_DB` | `0` | номер БД Redis |
| `CACHE_TTL_SECONDS` | `10` | TTL кэша списка |
| `RATE_LIMIT_RPM` | `120` | лимит запросов в минуту на IP к `/api/*` |
| `SHUTDOWN_TIMEOUT_SECONDS` | `15` | время на завершение активных запросов |

Некорректное значение — ошибка старта, а не тихий откат к умолчанию. В Kubernetes несекретное
приходит из ConfigMap, `POSTGRES_DSN` и `REDIS_PASSWORD` — из Secret, который External Secrets
собирает из Vault. Пример для локального запуска — [.env.example](.env.example).

## Надёжность

- **Graceful shutdown.** По `SIGTERM` сервер перестаёт принимать соединения и дожидается
  активных запросов. В кластере перед этим `preStop` даёт поду выйти из эндпоинтов Service.
- **Миграции** идемпотентны и защищены advisory lock — параллельный старт реплик безопасен.
- **Таймауты HTTP-сервера** и ограничение размера тела защищают от медленных клиентов
  и больших запросов.
- **IP клиента** берётся только из `X-Real-IP`, который ingress-nginx всегда перезаписывает;
  `X-Forwarded-For` от клиента не учитывается — обойти rate limit подменой заголовка нельзя.
- **Ошибки сервера** клиенту отдаются без деталей (SQL, сеть) — подробности только в логе.

## Локальный запуск

```bash
docker compose up --build
```

Приложение — http://localhost:8080, метрики — http://localhost:9090/metrics. Сначала
выполняется одноразовый сервис `migrate`, `app` стартует только после его успешного завершения.

## Тесты

```bash
go test -race ./...

# интеграционные — против настоящих Postgres и Redis (тест создаёт и удаляет свою БД)
docker compose up -d postgres redis
TEST_POSTGRES_DSN='postgres://todoapp:todoapp@localhost:5432/todoapp?sslmode=disable' \
TEST_REDIS_ADDR=localhost:6379 go test -tags integration -race ./internal/integration/
```

Интеграционные тесты покрывают миграции (повторный запуск), CRUD репозитория, кэш,
rate limit (429, TTL ключа), HTTP API целиком и `/readyz` при недоступном Redis.

## CI/CD и окружения

Окружения — не ветки, а values одного чарта [`charts/todo`](../charts/todo):

| Окружение | Адрес | Namespace | Values | Обновление |
|---|---|---|---|---|
| dev | http://todo-dev.192.168.0.15.nip.io | `todo-dev` | `values-dev.yaml` | автоматически после merge в `main` |
| prod | http://todo.192.168.0.15.nip.io | `todo-prod` | `values-prod.yaml` | git-тег `vX.Y.Z` + ручное подтверждение |

Ветки: `feature/*` → MR → `main`. Образ собирается один раз и продвигается dev → prod:
в prod попадает ровно тот образ, что работал на dev.

Пайплайн — [`ci/app.yml`](ci/app.yml):

| Стадия | Джоба | Что делает |
|---|---|---|
| lint | `app_lint` | `gofmt`, `go vet` |
| test | `app_test` | unit-тесты с `-race` |
| test | `app_integration_test` | интеграционные тесты, Postgres и Redis как services джобы |
| build | `app_build` | kaniko: `backend:<sha>`, для тега ещё `backend:vX.Y.Z` |
| scan | `app_scan` | trivy: HIGH/CRITICAL с доступным исправлением блокируют деплой |
| deploy | `app_deploy_dev` | `main`: тег `<sha>` в `values-dev.yaml` |
| deploy | `app_deploy_prod` | тег `vX.Y.Z`, вручную: тег в `values-prod.yaml` |

Деплой — GitOps: джоба коммитит новый тег в `main` (push с `-o ci.skip`), ArgoCD
(ApplicationSet `todo`) видит изменение и выкатывает его.

```bash
# релиз
git tag v1.2.0 && git push origin v1.2.0   # затем app_deploy_prod в пайплайне тега

# откат prod
git revert <коммит "deploy(prod): todo vX.Y.Z">
```

Для деплоя нужна CI/CD-переменная `APP_DEPLOY_TOKEN` — project access token с ролью
Maintainer (push в защищённую `main`) и scope `write_repository`.

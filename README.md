# trip-service

Сервис поездок TripGo, лабораторная работа 2: архитектура, качество кода и observability.

Снаружи сервис умеет то же, что после работы 1: создать, получить и завершить поездку.
Внутри код разложен по слоям с зависимостями, направленными внутрь, покрыт unit- и
интеграционными тестами, проходит `golangci-lint` и отдаёт логи, метрики и трейсы через
OpenTelemetry. По одному запросу видно запись в логе, трейс в Jaeger и точку на графике в Grafana.

## Требования

- Go 1.26+ (версия из `go.mod`; `oapi-codegen`, `goose`, `mockgen` ставятся в модуль и запускаются через `go tool`)
- `tripgoctl` — утилита курса для локального окружения
- `golangci-lint` v2 — отдельным бинарём, нужен для `make lint`

## Запуск

```bash
tripgoctl cluster start          # один раз на машине
tripgoctl environment start      # PostgreSQL + коллектор телеметрии, создаёт .env
tripgoctl connect                # ссылки на Jaeger, Grafana, Prometheus
make migrate                     # накатить миграции
make run                         # запустить сервис
```

Makefile читает переменные из `.env.example`, а затем из `.env` поверх него. `.env`
создаёт `tripgoctl` при каждом `environment start`: из него берутся актуальные адреса БД
и коллектора, а всё, чего в нём нет (`HTTP_*_TIMEOUT`, `OTEL_*`), — из `.env.example`.
Если запускать бинарь мимо `make`, экспортируйте оба файла по очереди: сначала
`.env.example`, потом `.env`.

## Цели Makefile

| Цель | Что делает |
|---|---|
| `make run` | запуск сервиса |
| `make build` | сборка в `bin/trip-service`, версия из `git describe` попадает в `service.version` |
| `make migrate` | накатить все миграции |
| `make migrate-down` | откатить одну последнюю миграцию |
| `make migrate-reset` | откатить все миграции |
| `make generate` | вся кодогенерация: OpenAPI → `internal/generated`, моки через `go generate` |
| `make test` | unit-тесты с `-race`, в конце печатает покрытие `usecase` + `domain` |
| `make test-integration` | тесты с тегом `integration` на реальном PostgreSQL |
| `make lint` | `golangci-lint` с конфигом `.golangci.yml` |

## Тесты

```bash
make test                                   # unit, без внешних зависимостей
tripgoctl environment start && make migrate # интеграционным нужна живая база со схемой
make test-integration
```

- Unit-тесты (`make test`) не требуют ни базы, ни сети: внешние зависимости заменены
  моками `gomock`. Моки генерируются `mockgen` через `//go:generate` в `contract.go`
  и лежат в `mocks_test.go` рядом с кодом.
- Интеграционные тесты помечены `//go:build integration` и работают с базой из
  `tripgoctl environment start`, адрес берут из `DATABASE_URL`. Каждый тест создаёт
  свои поездки со случайными `id` и удаляет их в `t.Cleanup`, поэтому тесты не мешают
  друг другу и проходят при повторном запуске (`-count=2`).
- Покрытие считается по `internal/usecase/...` и `internal/domain/...` (`-coverpkg`),
  итоговая цифра печатается в конце `make test`.

Обязательные сценарии и где они проверяются:

| Сценарий | Тест |
|---|---|
| создание поездки — успех | `usecase/trip/create`, `http/trip/create`, `http/router` |
| занятый водитель → `ErrDriverBusy` → `409` | `usecase/trip/create`, `http/trip/create`, `repository/trip` (`Create_DriverBusy`) |
| завершение активной поездки | `domain/trip` (`Finish`), `usecase/trip/finish`, `http/trip/finish` |
| завершение завершённой → `ErrTripCompleted` → `409` | `domain/trip`, `usecase/trip/finish`, `http/trip/finish` |
| получение несуществующей → `404` | `usecase/trip/get`, `http/trip/get`, `repository/trip` |
| невалидное тело → `400 invalid_request` | `http/trip/create` (`InvalidBody`), `http/router` (невалидный `tripId`) |
| ошибка репозитория → `500` без деталей в теле | `http/trip/*` (`repository error`), `http/domainerror` |
| `INSERT` в двух таблицах откатывается целиком | `infra/postgres/transaction` (`RollbackOnSecondInsertError`) |
| два параллельных `finish` → один `200`, один `409` | `repository/trip` (`Finish_Concurrent`) |

## Переменные окружения

Пример значений лежит в `.env.example`. Сервис падает на старте с ошибкой, в которой
названа переменная, если:

- обязательная переменная не задана или не разбирается в нужный тип;
- любой таймаут (`HTTP_*_TIMEOUT`, `SHUTDOWN_TIMEOUT`, `DATABASE_*_TIMEOUT`,
  `DATABASE_MAX_CONN_LIFETIME`) не больше нуля;
- `DATABASE_MAX_CONNS <= 0`, `DATABASE_MIN_CONNS < 0` или `DATABASE_MIN_CONNS > DATABASE_MAX_CONNS`;
- `HTTP_ADDR` или `DATABASE_URL` пустые;
- `LOG_LEVEL` не из `debug`, `info`, `warn`, `error`.

| Переменная | Назначение |
|---|---|
| `HTTP_ADDR` | адрес HTTP-сервера |
| `HTTP_READ_TIMEOUT` | таймаут чтения запроса целиком |
| `HTTP_READ_HEADER_TIMEOUT` | таймаут чтения заголовков |
| `HTTP_WRITE_TIMEOUT` | таймаут записи ответа |
| `HTTP_IDLE_TIMEOUT` | таймаут простоя keep-alive соединения |
| `SHUTDOWN_TIMEOUT` | общий бюджет на остановку: сервер, пул, сброс телеметрии |
| `LOG_LEVEL` | уровень логов `slog` |
| `DATABASE_URL` | строка подключения к PostgreSQL |
| `DATABASE_MAX_CONNS`, `DATABASE_MIN_CONNS` | размер пула |
| `DATABASE_MAX_CONN_LIFETIME` | время жизни соединения в пуле |
| `DATABASE_CONNECT_TIMEOUT` | таймаут подключения, Ping на старте и в `/ready` |
| `DATABASE_QUERY_TIMEOUT` | таймаут одного запроса в БД |
| `OTEL_SERVICE_NAME` | `service.name` в телеметрии и поле `service` в логах, по умолчанию `trip-service` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | адрес коллектора, приходит из `.env` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` или `http/protobuf` |
| `OTEL_TRACES_SAMPLER` | сэмплер трейсов, локально `parentbased_always_on` |
| `OTEL_RESOURCE_ATTRIBUTES` | `service.namespace=tripgo,deployment.environment=local` |
| `OTEL_METRIC_EXPORT_INTERVAL` | период отправки метрик в коллектор, мс |

`HTTP_*_TIMEOUT` добавлены сверх базового набора курса: conventions запрещают хардкодить
таймауты. `OTEL_*` — стандартные переменные OpenTelemetry SDK, их читает сам SDK
(`autoexport`, `resource.WithFromEnv`), своих имён не заводили.

## API

| Метод | Путь | Ответы |
|---|---|---|
| POST | `/api/v1/trips` | 201, 400, 409, 500 |
| GET | `/api/v1/trips/{tripId}` | 200, 400, 404, 500 |
| POST | `/api/v1/trips/{tripId}/finish` | 200, 400, 404, 409, 500 |
| GET | `/health` | 200 |
| GET | `/ready` | 200, 503 |

Ошибки — RFC 9457, `application/problem+json`, клиент разбирает поле `code`:
`invalid_request`, `trip_not_found`, `trip_completed`, `driver_busy`, `internal_error`.

## Архитектура

### Слои

```
cmd/service/main.go          читает конфиг, зовёт container.New, запускает сервер, ждёт сигнала
internal/
  config/                    разбор и валидация env
  container/                 единственное место сборки: pgxpool.New, репозитории, usecase, хендлеры
  domain/trip/               Trip, статусы, переход Finish, валидация, доменные ошибки
  model/                     входные структуры между транспортом и usecase
  usecase/trip/{create,get,finish}/   сценарии; contract.go с интерфейсами зависимостей
  http/
    trip/{create,get,finish}/         хендлер на ручку, convert.go: api ↔ домен
    health/                           /health и /ready
    domainerror/                      единый маппинг «ошибка → Problem»
    middleware/                       Recover, Tracing, Logging, Metrics
    router/                           chi + сгенерированный ServerInterface
  repository/trip/           pgx + squirrel, ошибки pgx → доменные ошибки
  infra/
    postgres/                пул pgx с otelpgx
      transaction/           менеджер транзакций
    telemetry/               OTel SDK, slog с trace_id/span_id, бизнес-метрики
  generated/                 код по OpenAPI (пакет api)
migrations/
```

Кто от кого зависит:

```
            ┌──────────── container ────────────┐
            ▼                 ▼                 ▼
          http ──────────► usecase ◄────── repository ──► infra/postgres
            │                 │                 │
            └──────► model, domain ◄────────────┘
```

- `domain` не импортирует никого из сервиса и ничего инфраструктурного.
- `usecase` знает только `domain`, `model` и собственные интерфейсы. Ни `pgx`, ни `net/http`.
- `http` знает `usecase` только через свои интерфейсы, плюс `domain` (для маппинга ошибок)
  и `generated`. В `repository` и `infra` не ходит.
- `repository` знает `domain` и `infra/postgres/transaction` (достаёт транзакцию из контекста).
- Конкретные типы встречаются друг с другом только в `container`.

Правила закреплены `depguard` в `.golangci.yml`: нарушение ломает `make lint`, а не
только договорённость. Дополнительно можно проверить так:

```bash
go list -deps ./internal/domain/... | grep -E 'jackc/pgx|go-chi|net/http' && echo "НАРУШЕНИЕ"
```

### Где объявлены интерфейсы и почему там

Интерфейсы объявляет потребитель, в `contract.go` своего пакета, неэкспортируемыми:

- `usecase/trip/create` объявляет `tripRepository` (`Create`, `AddStatusHistory`),
  `txManager` и `tripEvents`; `usecase/trip/finish` — свой `tripRepository`
  (`GetByIDForUpdate`, `Save`, `AddStatusHistory`); `usecase/trip/get` — `GetByID`.
- `http/trip/*` объявляют интерфейс своего usecase (`tripCreator`, `tripGetter`,
  `tripFinisher`), `http/health` — `pinger`.

Так каждый пакет описывает ровно то, что ему нужно: интерфейсы получаются узкими,
мок на них маленький, а `repository/trip.Repository` удовлетворяет всем трём
интерфейсам, ничего о них не зная. Когда в работе 4 появится gRPC, новый транспорт
объявит такой же интерфейс usecase у себя и пойдёт в тот же usecase; usecase, домен и
репозиторий не меняются, добавится только маппинг доменной ошибки в gRPC-код.

### Ошибки

- Доменные ошибки (`ErrTripNotFound`, `ErrTripCompleted`, `ErrDriverBusy`) и тип
  `ValidationError` с полем и описанием лежат в `domain/trip/errors.go`.
- Репозиторий переводит `pgx.ErrNoRows` в `ErrTripNotFound`, а `23505` по индексу
  `idx_one_active_trip_per_driver` — в `ErrDriverBusy`. Остальные ошибки оборачиваются
  с названием операции (`create trip: add status history: ...`).
- Хендлер передаёт ошибку в `domainerror.Write`, не разбирая её. `switch` по
  `errors.Is`/`errors.As` написан один раз, в `domainerror.Problem`.
- Для `5xx` тело содержит только `code=internal_error`. Сама ошибка попадает в спан
  (`RecordError`, `codes.Error`, `error.type`) и в лог запроса: `domainerror` кладёт её
  в слот контекста, а middleware логирования достаёт и пишет в запись уровня `Error`.
  Так одна ошибка логируется один раз — там, где запрос завершился.

### Отступления от раскладки из задания

- `cmd/service/` вместо `cmd/trip-service/` из `conventions.md` — так в целевой
  структуре работы 2.
- Код OpenAPI лежит в `internal/generated/`, как в целевой структуре. Сюда же его
  кладёт `make generate`, а правила `depguard` для домена и usecase ссылаются на этот путь.

## Observability

Сервис пишет метрики и трейсы через OpenTelemetry SDK и отправляет их по OTLP в коллектор,
который раскладывает их в Prometheus и Jaeger. В коде нет ни Prometheus, ни Jaeger.
Если коллектор недоступен, ошибки экспорта уходят в лог `Warn`, а сервис отвечает штатно.

Атрибуты ресурса: `service.name=trip-service`, `service.version` (из сборки),
`service.namespace=tripgo`, `deployment.environment=local`.

### Middleware

```
Recover → Tracing → Logging → Metrics → роутер
```

- **Recover** ловит панику, пишет `Error` со стектрейсом, отдаёт `500 internal_error`.
  `http.ErrAbortHandler` пробрасывает дальше, как это делает `net/http`.
- **Tracing** — `otelhttp`, корневой спан на запрос. После роутинга спан переименовывается
  в `{method} {route}` (`POST /api/v1/trips`) и получает `http.route` с шаблоном.
  Собственные метрики `otelhttp` отключены, чтобы не дублировать наши.
- **Logging** — одна запись `request handled` на запрос, когда ответ готов: `method`,
  `route` (шаблон), `status`, `duration_ms`. `Info` для `2xx`/`4xx`, `Error` для `5xx`.
  Тела запроса и ответа не логируются.
- **Metrics** — подключена как middleware сгенерированного роутера, поэтому шаблон
  маршрута уже известен.

### Метрики

| Метрика | Тип | Лейблы | Где пишется |
|---|---|---|---|
| `http.server.request.duration` | histogram, с | `http.request.method`, `http.route`, `http.response.status_code` | `middleware.NewMetrics` |
| `http.server.active_requests` | up-down counter | `http.request.method`, `http.route` | `middleware.NewMetrics` |
| `db.client.operation.duration` | histogram, с | `db.operation.name`, `db.collection.name`, `error.type` | `otelpgx` на уровне пула |
| `tripgo.trips.created` | counter | — | `telemetry.TripEvents`, после коммита |
| `tripgo.trips.completed` | counter | — | `telemetry.TripEvents`, после коммита |

В `http.route` кладётся только шаблон (`/api/v1/trips/{tripId}`). С `tripId` каждая поездка
завела бы свою временную серию: кардинальность растёт без границ, Prometheus раздувается,
а метрика перестаёт агрегироваться по ручке. Запросы на несуществующий путь получают
`route=unmatched`.

Отдельных счётчиков бизнес-ошибок нет: `driver_busy` и `trip_not_found` видны в
`http.server.request.duration` по `http.response.status_code`.

В Prometheus имена приходят с точками, заменёнными на `_`, и суффиксами единиц:
`http_server_request_duration_seconds_bucket`, `http_server_active_requests`,
`db_client_operation_duration_seconds_bucket`, `tripgo_trips_created_total`,
`tripgo_trips_completed_total`.

### Трейсы

- Корневой спан — входящий HTTP-запрос.
- Каждый запрос в БД — дочерний спан от `otelpgx`, подключённого как `Tracer` пула pgx.
  Инструментирование на уровне пула, а не в методах репозитория: ни один запрос не
  забудут обернуть, а репозиторий не зависит от телеметрии.
- Ошибка `5xx` помечает спан `codes.Error` и пишет `error.type`.
- Контекст пробрасывается по `W3C TraceContext`.

### Логи

`log/slog`, JSON, stdout. Логгер создаётся в `container` и передаётся зависимостью,
`slog.Default()` в коде не используется. Обёртка `traceHandler` достаёт `SpanContext`
из `ctx` записи и добавляет `trace_id` и `span_id`, поэтому они есть во всех записях,
сделанных через `*Context`-методы внутри спана: по `trace_id` из лога находится трейс в
Jaeger. Поле `service` добавляется ко всем записям.

Бизнес-события `trip created` и `trip completed` пишет `TripEvents` после успешного
коммита, в лог попадает только `trip_id`: id пассажира и водителя — персональные данные.

### Как проверить глазами

```bash
make run
curl -sS -X POST localhost:8080/api/v1/trips -H 'content-type: application/json' -d @trip.json -i
```

- Jaeger (ссылка из `tripgoctl connect`): сервис `trip-service`, трейс `POST /api/v1/trips`,
  внутри спаны `INSERT` в `trips` и `trip_status_history`.
- Grafana: `http_server_request_duration_seconds_bucket`, `tripgo_trips_created_total`.
- Взять `trace_id` из записи `request handled` и найти по нему трейс в Jaeger.
- Остановить коллектор — сервис продолжает отвечать, в логе `Warn` об экспорте.

## Решения из работы 1

### Уровень изоляции: Read Committed

Задаётся явно при старте транзакции в `Do`. Инварианты держит сама база, а не изоляция:
запрет двух активных поездок — частичный уникальный индекс, от двойного завершения —
`SELECT ... FOR UPDATE`. Serializable ничего бы не добавил, но потребовал бы повторов
при serialization failure.

### Менеджер транзакций

`transaction.Manager.Do(ctx, fn)`:

- открывает транзакцию, кладёт её в `context` под приватным ключом, вызывает `fn`;
- `fn` вернула `nil` — `COMMIT`; ошибку или паника — `ROLLBACK` через `defer`, ошибка или
  паника уходят наружу;
- репозиторий на каждом вызове достаёт исполнителя из контекста: есть транзакция —
  работает через неё, нет — через пул. Вне `Do` запрос просто идёт через пул;
- вложенный `Do` видит транзакцию в контексте и не открывает вторую.

Usecase знает менеджер только через свой интерфейс `txManager`, про pgx не знает.

### Запрет двух активных поездок у водителя

```sql
CREATE UNIQUE INDEX idx_one_active_trip_per_driver ON trips (driver_id) WHERE status = 'active';
```

Из параллельных `INSERT` пройдёт один, остальные получат `23505`, которое репозиторий
превращает в `ErrDriverBusy`, а транспорт — в `409 driver_busy`. 20 параллельных создания
на одного водителя дают один `201` и 19 `409`.

### Завершение поездки при конкурентных запросах

`finish` — один `Do`: `SELECT ... FOR UPDATE`, переход `trip.Finish` в домене, `UPDATE`,
запись в журнал. Второй параллельный запрос ждёт блокировку строки, после коммита первого
читает уже `completed`, домен возвращает `ErrTripCompleted` → `409`. `finished_at` не
перезаписывается. Проверяется интеграционным тестом `TestFinish_Concurrent`.

### Валидация тела запроса

Тело `POST /api/v1/trips` читается целиком (до 1 МБ), ключи сверяются с допустимыми по
точному совпадению на верхнем уровне и внутри точек, затем тело разбирается в структуру с
полями-указателями, чтобы отличить отсутствующее поле от законного нуля. Диапазоны
координат, `price >= 0` и непустые UUID проверяет конструктор `trip.New` в домене.
Любая ошибка — `400 invalid_request` с полем в `detail`.

### Graceful shutdown

По `SIGINT`/`SIGTERM` один контекст на `SHUTDOWN_TIMEOUT` на всё завершение:
`server.Shutdown` (по истечении — `Close`), затем закрытие пула, затем сброс
телеметрии. Сервер останавливается раньше пула, потому что текущим запросам нужна БД.
Если бюджет кончился, в лог пишется `forced shutdown`, процесс выходит с кодом 1.

## Известные упрощения

- Задание со звёздочкой (мутационное тестирование) не сделано.
- Задания со звёздочкой из работы 1 (идемпотентность, Dockerfile) не сделаны.

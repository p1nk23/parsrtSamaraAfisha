# checker parser-service

Отдельный Go-сервис для импорта событий с Яндекс Афиши в формат фронтенда (`ApiEventFull` + `eventSessions`) с сохранением в PostgreSQL.

## Что умеет

- `POST /parse/yandex-afisha` парсит Яндекс Афишу и сохраняет найденные события в PostgreSQL.
- `GET /events` и `GET /event` возвращают последние сохраненные события для фронта.
- `GET /health` показывает статус, URL источника и режим хранения.
- Если нужных данных нет в обычном HTML, сервис открывает страницу через headless Chromium.
- Есть CORS для локального фронта.

## Быстрый запуск через Docker Compose

```bash
cd parser-service
docker compose up --build
```

Проверка:

```bash
curl http://localhost:8081/health
curl -X POST http://localhost:8081/parse/yandex-afisha
curl http://localhost:8081/events
```

В compose поднимаются:

- `parser-service` на `localhost:8081`;
- `postgres` на `localhost:5432`.

Данные БД хранятся в Docker volume `postgres_data`.

## API

### `GET /health`

Пример:

```json
{
  "status": "ok",
  "service": "checker-parser-service",
  "storageMode": "postgres",
  "useBrowser": true
}
```

### `POST /parse/yandex-afisha`

Запускает парсинг, сохраняет результат в PostgreSQL и возвращает этот же результат.

```bash
curl -X POST http://localhost:8081/parse/yandex-afisha
```

Можно передать другой URL:

```bash
curl -X POST "http://localhost:8081/parse/yandex-afisha?url=https://afisha.yandex.ru/samara/selections/hot?source=selection-events&city=samara"
```

### `GET /events`

Возвращает события из PostgreSQL:

```bash
curl http://localhost:8081/events
```

`GET /event` оставлен как alias для совместимости с текущим фронтом.

## Локальный запуск без Docker

Для режима PostgreSQL нужен установленный `psql`, потому что этот прототип использует PostgreSQL CLI без внешних Go-драйверов.

```bash
export DATABASE_URL="postgres://checker:checker@localhost:5432/checker_events?sslmode=disable"
export PSQL_BIN="psql"
go run ./cmd/parser
```

Если `DATABASE_URL` не задан, сервис автоматически переключится на файловое хранение `data/events.json`.

## Переменные окружения

| Переменная | Значение по умолчанию | Описание |
|---|---:|---|
| `HTTP_ADDR` | `:8081` | адрес HTTP-сервера |
| `DATABASE_URL` | пусто | если задано, используется PostgreSQL |
| `DATA_PATH` | `data/events.json` | fallback-файл, если `DATABASE_URL` не задан |
| `USE_BROWSER` | `true` | включить fallback через Chrome/Chromium |
| `BROWSER_BIN` | авто-поиск | путь к Chrome/Chromium |
| `BROWSER_TIMEOUT` | `45s` | timeout браузерного рендера |
| `PSQL_BIN` | `psql` | путь к PostgreSQL CLI |

## Тесты

```bash
go test ./...
```

Покрыто тестами:

- HTTP helpers и recover middleware;
- CORS preflight;
- парсинг JSON-LD событий;
- fallback-парсинг карточек из HTML;
- нормализация текста и URL;
- файловое хранилище;
- SQL escaping helpers для PostgreSQL-прототипа.

## Важное ограничение

Сейчас PostgreSQL-слой сделан через `psql`, чтобы не добавлять внешние Go-зависимости в прототип. Для production лучше заменить `internal/storage/postgres.go` на `pgx` + `sqlc` или `database/sql` + драйвер.

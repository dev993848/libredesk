# Кабинет клиента и учёт времени — дизайн MVP

> Статус: черновик для обсуждения
> Дата: 2026-11-13
> Ветка: `main` (форк)

## 0. Статус реализации

Реализовано в ветке `feature/customer-portal`:

- [x] миграция `v2.11.0`: `portal_login_tokens`, `conversation_time_entries`, колонки времени в `conversations`
- [x] magic-link вход (одноразовый токен, 15 минут) и JWT SSO
- [x] отдельная портальная сессия (`libredesk_portal`, Redis, 30 дней) + CSRF
- [x] API: `me`, список, карточка, отметка прочтения, ответ, новое обращение, вложения
- [x] SPA `/portal`: вход, список, карточка, ответ, новое обращение, вложения
- [x] учёт времени: таймер, ручные записи, суммы, пересчёт `conversations.time_spent_seconds`
- [x] агентский блок «Учёт времени» в сайдбаре обращения
- [x] права: `time_tracking:manage` у роли Admin
- [x] тесты: одноразовость токена, скоуп по контакту, таймер/суммы, портальные сессии
- [x] карточка «Затраченное время» в отчётах (итог, billable, топ агентов)
- [x] настройки портала в админке: включение, инбокс, JWT-секрет, сроки
- [x] шаблон письма `static/email-templates/portal-magic-link.html`
- [x] тимлид ограничен обращениями своей команды: право `time_tracking:manage` + проверка доступа к обращению (командные права `conversations:read_team_*`)
- [x] Cypress API-тесты настроек портала и отчёта по времени

Осталось до полного MVP:

- [ ] тесты хендлеров портала (Go) и e2e сценарии UI
- [ ] отдельный домен портала (нужны cookie-домен, CORS и `portal.root_url`)



## 1. Цель

Дать клиенту (контакту) личный кабинет, в котором он:

- видит **все свои обращения** по всем каналам (email, live chat, WhatsApp);
- видит **статус** каждого обращения (открыт / ожидает / решён / закрыт);
- читает переписку и может ответить;
- видит, **сколько времени** команда потратила на обращение.

Агенты при этом ведут учёт времени: таймер и/или ручные записи в карточке обращения.

## 2. Что есть сейчас

| Возможность | Состояние |
|---|---|
| Аккаунт клиента | Нет. Контакты — записи `users` с `type = 'contact'` без пароля и логина |
| Список обращений клиента | Только виджет живого чата, только по одному инбоксу (`GetContactChatConversations(contactID, inbox.ID)`) |
| Статусы | Есть в данных (`conversation_statuses`), клиенту не показываются |
| Переписка | Виджет (WS), email, ссылки из писем |
| Учёт времени | Отсутствует полностью. Есть только SLA-метрики (первый ответ / решение) в отчётах |
| Портал клиента | В `ROADMAP.md` числится как In Progress |

Вывод: нужен новый публичный интерфейс и новый домен учёта времени.

## 3. Ключевые решения

Подтверждено заказчиком:

1. **Только заранее заведённые контакты.** Самостоятельной регистрации нет; magic link уходит только существующему контакту с email.
2. **Клиент видит только итог времени** по обращению. Разбивка по записям и агентам — только в админке.
3. **Billable-учёт** — поле в схеме заводим сразу (`billable`, по умолчанию `true`), но в MVP не показываем и не считаем отдельно. Это дешевле, чем добавлять колонку позже.
4. **Ответы и создание новых обращений** — в MVP.
5. **JWT SSO** — в MVP, вместе с magic link.
6. **Вложения** — в обе стороны, в MVP.
7. **Портал на `/portal`**.
8. **Правка чужих записей времени** — админ и «тимлид». Отдельной сущности тимлида в Libredesk нет, поэтому делаем через кастомную роль с правом `time_tracking:manage`; правка разрешена только по обращениям, к которым у пользователя есть доступ (`authz.EnforceConversationAccess`).

Технические решения:

1. **Отдельное SPA `apps/portal`** на `/portal`, а не страница внутри агентского приложения.
   Агентский SPA завязан на сессию агента, права и `/api/v1/*`; смешивать публичный и внутренний контуры в одном бандле рискованно.
2. **Отдельные API `/api/v1/portal/*` и отдельная сессия** (`libredesk_portal`, префикс Redis `portal_sess:`). Агентская сессия и `auth`-middleware не переиспользуются.
3. **Авторизация — magic link по email** (основной сценарий) + **JWT SSO** (в MVP) — те же claims, что у виджета (`external_user_id` + `email` + подпись секретом инбокса).
4. **Модель данных переиспользуется**: `users` (контакты), `conversations`, `conversation_messages`, `media`, `conversation_statuses`. Новые таблицы: `portal_login_tokens`, `conversation_time_entries`.
5. **Суммы времени денормализуются** в `conversations`, чтобы список не считал `SUM()` по каждой строке.
6. **Приватность по умолчанию**: портал никогда не отдаёт внутренние заметки, activity-события, назначения, теги и служебные поля. Каждый запрос фильтруется по `contact_id` из сессии.

### 3.1 Новые обращения из портала

Клиент создаёт обращение в **настраиваемый инбокс** (новая настройка `portal.default_inbox_id`).
Рекомендация для MVP — email-инбокс: ответ агента уходит клиенту и на почту, и в портал, ничего дополнительно изобретать не нужно.
Если настроен live chat-инбокс, обращение живёт как чат без email-уведомлений.

Будущее расширение — отдельный канал `portal` (без внешнего транспорта). В MVP не делаем, чтобы не плодить новый тип канала.

### 3.2 Что уже есть в апстриме

Портал в Libredesk пытались делать несколько раз — это можно переиспользовать вместо написания с нуля:

| Что | Статус | Чем полезно |
|---|---|---|
| Issue #221 «Customer web portal» | открыт | Постановка задачи |
| Issue #635 «Customer-scoped API access for external support portals» | открыт | Требования к безопасности: только свои обращения, без private notes, аудит источника auth |
| PR #493 «OIDC customer ticket portal» | закрыт, не влит (авг. 2026) | Готовый портал: contact-only auth middleware, CSRF, rate limit, безопасные DTO, список/карточка/новое обращение, UI `PortalView.vue` (~1172 строки) |
| PR #638 «Customer-scoped API for trusted external applications» | закрыт, не влит (нояб. 2026) | API `/api/v1/portal`: список, чтение, создание, ответы, вложения, тесты; авторизация по portal API key + `X-Portal-External-User-ID` (~668 строк) |

Вывод: **базу портала разумно port'ить из #493 (UI + auth) и #638 (API), а не писать заново.**
Оба PR закрыты и устарели относительно `main`, поэтому их код — источник решений и заготовок, а не готовый патч:

- #493 завязан на OIDC и старые миграции `v2.7.0/v2.8.1/v2.8.2`; нам нужны magic link + JWT и актуальные миграции.
- #638 создаёт контакт при первом обращении и использует API-key вместо клиентской сессии; нам нужно «только заранее заведённые контакты» и браузерную сессию.

Перед реализацией: вытащить diff'ы PR, провести security review (особенно скоуп по `contact_id`), перенести DTO/запросы, заменить auth-слой на magic link + JWT.

## 4. Модель данных

Миграция `v2.11.0` (файл `internal/migrations/v2.11.0.go`, регистрация в `cmd/upgrade.go`), плюс те же таблицы в `schema.sql` для чистых установок.

```sql
-- Одноразовые токены для входа по email.
CREATE TABLE IF NOT EXISTS portal_login_tokens (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    contact_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,          -- sha256(raw token), сам токен не храним
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ NULL,
    requested_ip TEXT NULL
);
CREATE INDEX IF NOT EXISTS index_portal_login_tokens_on_contact_id
    ON portal_login_tokens (contact_id);
CREATE INDEX IF NOT EXISTS index_portal_login_tokens_on_expires_at
    ON portal_login_tokens (expires_at);

-- Записи учёта времени по обращению.
CREATE TABLE IF NOT EXISTS conversation_time_entries (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NULL,                 -- NULL = таймер идёт
    duration_seconds INT NOT NULL DEFAULT 0,
    note TEXT NULL,
    billable BOOL NOT NULL DEFAULT TRUE,
    CONSTRAINT constraint_time_entries_duration
        CHECK (duration_seconds >= 0 AND duration_seconds <= 86400)
);
CREATE INDEX IF NOT EXISTS index_time_entries_on_conversation_id
    ON conversation_time_entries (conversation_id);
CREATE INDEX IF NOT EXISTS index_time_entries_on_user_id
    ON conversation_time_entries (user_id);
-- У одного агента одновременно может идти только один таймер.
CREATE UNIQUE INDEX IF NOT EXISTS index_time_entries_one_running_per_user
    ON conversation_time_entries (user_id) WHERE ended_at IS NULL;

-- Денормализованные суммы для быстрых списков.
ALTER TABLE conversations
    ADD COLUMN IF NOT EXISTS time_spent_seconds INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS billable_time_seconds INT NOT NULL DEFAULT 0;
```

Правила пересчёта: любая вставка/правка/удаление записи в той же транзакции пересчитывает обе колонки обращения:

```sql
UPDATE conversations c SET
    time_spent_seconds = COALESCE((SELECT SUM(duration_seconds)
        FROM conversation_time_entries WHERE conversation_id = c.id), 0),
    billable_time_seconds = COALESCE((SELECT SUM(duration_seconds)
        FROM conversation_time_entries WHERE conversation_id = c.id AND billable), 0)
WHERE c.id = $1;
```

Инварианты:

- `ended_at != NULL` ⇒ `duration_seconds > 0`.
- Таймер (`ended_at IS NULL`) не учитывается в суммах до остановки.
- Максимальная длительность одной записи — 24 часа (защита от забытого таймера); при превышении таймер останавливается на 24 ч и помечается в `note`/логе.
- Записи времени удаляются каскадно вместе с обращением.

## 5. API

Все ответы — в существующем формате `envelope` (`{status, data, message, error_type}`).

### 5.1 Публичные (клиентские) эндпоинты

| Метод | Путь | Назначение |
|---|---|---|
| POST | `/api/v1/portal/auth/magic-link` | Запросить ссылку входа по email. Всегда `200`, без раскрытия существования контакта |
| GET | `/api/v1/portal/auth/verify?token=…` | Проверить токен и показать страницу подтверждения (токен не сжигается) |
| POST | `/api/v1/portal/auth/verify` | Погасить токен, поставить сессию, `302` на `/portal` |
| POST | `/api/v1/portal/auth/exchange` | Обмен JWT на сессию |
| POST | `/api/v1/portal/auth/logout` | Удалить сессию |
| GET | `/api/v1/portal/me` | Контакт: имя, email, аватар |
| GET | `/api/v1/portal/conversations` | Список: `status`, `q`, `page`, `per_page` |
| GET | `/api/v1/portal/conversations/{uuid}` | Карточка: сообщения, вложения, статус, время |
| POST | `/api/v1/portal/conversations/{uuid}/messages` | Ответ клиента |
| POST | `/api/v1/portal/conversations` | Создать обращение в `portal.default_inbox_id` |
| POST | `/api/v1/portal/conversations/{uuid}/seen` | Отметить прочитанным |
| GET | `/api/v1/portal/conversations/{uuid}/transcript` | Скачать переписку (опционально) |

Пример ответа списка:

```json
{
  "status": "success",
  "data": {
    "results": [
      {
        "uuid": "…",
        "reference_number": "ABC-1234",
        "subject": "Не проходит оплата",
        "status": { "name": "Open", "category": "open" },
        "inbox_name": "Поддержка",
        "created_at": "2026-11-01T10:00:00Z",
        "last_message_at": "2026-11-02T12:30:00Z",
        "last_message_preview": "Проверьте, пожалуйста…",
        "unread_count": 2,
        "time_spent_seconds": 5400,
        "billable_time_seconds": 3600
      }
    ],
    "total": 12
  }
}
```

Правила ответа зависят от канала и **переиспользуют существующую логику**, портал их не дублирует:

- live chat — настройка `prevent_reply_to_closed_conversation`;
- email — `reopen_window_hours` инбокса;
- WhatsApp — 24-часовое окно Meta.

При запрете — `409` с понятным текстом; UI прячет поле ответа.

Непрочитанные считаются по `conversations.contact_last_seen_at` (поле уже существует), эндпоинт `/seen` его обновляет. Дополнительная таблица не нужна.

### 5.2 Агентские эндпоинты учёта времени

| Метод | Путь | Право |
|---|---|---|
| POST | `/api/v1/conversations/{uuid}/timer/start` | `conversations:write` |
| POST | `/api/v1/conversations/{uuid}/timer/stop` | `conversations:write` |
| GET | `/api/v1/conversations/{uuid}/time-entries` | `conversations:read` |
| POST | `/api/v1/conversations/{uuid}/time-entries` | `conversations:write` |
| PUT | `/api/v1/time-entries/{id}` | автор или `time_tracking:manage` |
| DELETE | `/api/v1/time-entries/{id}` | автор или `time_tracking:manage` |

Новое право `time_tracking:manage` добавляется в `internal/authz` и в экран ролей.
Таймер стартует от имени текущего агента; остановка чужих таймеров запрещена.
Для «тимлида» отдельная сущность не нужна: админ создаёт кастомную роль (например, `Team Lead`), выдаёт ей `time_tracking:manage`, а правка чужих записей дополнительно проверяется через `authz.EnforceConversationAccess` — тимлид может править время только по доступным ему обращениям.

Клиенту в портале отдаётся **только итог** (`time_spent_seconds`). Разбивка по записям и агентам доступна только в админке.
Billable-колонка заводится в схеме, но в MVP не показывается и в отчётах не выделяется.

## 6. Авторизация и безопасность

### 6.1 Magic link

1. `POST /auth/magic-link {email}`.
2. Нормализуем email (`strings.ToLower`), ищем **уже существующий** контакт: `type='contact' AND deleted_at IS NULL AND LOWER(email)=$1`. Новых контактов не создаём.
3. Если контакт не найден или заблокирован — всё равно `200` (защита от перебора). В dev-режиме (`app.env=dev`) можно вернуть `debug_token` для e2e.
4. Генерируем 32 случайных байта (base64url), считаем `sha256`, пишем `portal_login_tokens` с TTL 15 минут. Храним только хеш.
5. Отправляем письмо со ссылкой `{app.root_url}/api/v1/portal/auth/verify?token=…`.
6. `verify` (GET): ищем по хешу, проверяем `expires_at > now()`, `used_at IS NULL`, контакт активен и не заблокирован. Токен **не сжигаем**, показываем страницу с кнопкой «Войти» — так почтовые сканеры ссылок не расходуют одноразовый токен.
7. `verify` (POST): ещё раз проверяем токен, ставим `used_at`, создаём сессию, `302` на `/portal`.
7. Повторное использование токена → `401`.

### 6.2 JWT SSO

- Новая настройка `portal.jwt_secret` (генерируется в админке, показывается один раз) и переключатель `portal.jwt_enabled`.
- `POST /api/v1/portal/auth/exchange {jwt}`: HMAC-подпись, обязательные claims `external_user_id`, `email`, `first_name`; опционально `last_name`, телефон, кастомные атрибуты. Тот же формат, что у виджета (`verifyStandardJWT`), но со своим секретом.
- Контакт ищется по `external_user_id`, затем по `LOWER(email)`. Если не найден — `401` (без автосоздания).
- Сессия выдаётся так же, как после magic link. JWT можно обменивать при каждом открытии портала — сессия продлевается.
- Если оба способа включены, на экране входа показываем email-форму; JWT — программный вход с сайта клиента.

### 6.3 Сессия

- Redis, префикс `portal_sess:`, TTL 30 дней (настройка).
- Cookie `libredesk_portal`: `HttpOnly`, `SameSite=Lax`, `Secure` при `app.server.disable_secure_cookies=false`, `Path=/`.
- Индекс сессий контакта `portal_user_sessions:<id>` — для отзыва при блокировке/удалении.
- CSRF: кука `csrf_token` + заголовок `X-CSRFTOKEN` для POST/PUT/DELETE (как у агентов).
- Middleware `portalAuth`: читает сессию, поднимает контакт, проверяет `enabled`, `deleted_at IS NULL`. Блокировка контакта в Libredesk — это `users.enabled = false` (см. `is-email-blocked`), отдельного флага нет.

### 6.4 Ограничения

- Новый лимитер `[rate_limit.portal]` (например, 100 req/min).
- Отдельный жёсткий лимит на `magic-link`: 5 запросов / 15 минут на email+IP.
- Загрузка вложений — через существующий `media` с проверкой владельца обращения.

### 6.5 Приватность (обязательные тесты)

- Любой запрос портала фильтруется `WHERE c.contact_id = $sessionContactID`.
- Сообщения: только `private = FALSE` и `type IN ('incoming','outgoing')`.
- Не отдавать: внутренние заметки, `activity`, назначенных агентов (кроме имени/аватара в сообщении), теги, SLA, приоритет, email агентов.
- Заблокированный контакт (`contacts.block`) не может войти и получает `401` на всех эндпоинтах.
- Контакт без email не может войти (только контакты с email; для WhatsApp-only — сообщение об ошибке входа).

## 7. Экраны

### 7.1 Портал (`frontend/apps/portal`)

| Экран | Содержимое |
|---|---|
| `/portal/login` | Email, кнопка «Отправить ссылку», сообщение «Проверьте почту» |
| `/portal` | Список обращений: бейдж статуса, тема/номер, превью, дата, непрочитанные, «Затрачено: 1 ч 30 мин»; фильтры «Все / Открытые / Решённые»; поиск; кнопка «Новое обращение» |
| `/portal/conversation/new` | Тема, сообщение, вложения; создаёт обращение в `portal.default_inbox_id` |
| `/portal/conversation/{uuid}` | Переписка (только клиентские сообщения), вложения, поле ответа, статус, итог времени |
| `/portal/profile` | Имя, email, выход |
| `/portal/error` | 404/403/истёкшая ссылка |

- Mobile-first, компоненты из `shared-ui`.
- Брендинг: `app.site_name`, `app.logo_url`, `app.favicon_url` из настроек.
- Языки — те же пакеты `i18n/*.json` (включая `ru-RU`).
- Если обращение закрыто и включено `prevent_reply_to_closed_conversation` — поле ответа скрыто, показывается пояснение.
- Показ времени — только итог по обращению, без разбивки по агентам и записям.
- Вложения: загрузка в новом обращении и в ответе, скачивание через существующие подписанные URL (`/uploads/{uuid}`).

### 7.2 Агентская часть

- В сайдбаре обращения блок «Учёт времени»: кнопка `Старт/Стоп`, текущий таймер, итог (`2 ч 15 мин`), список записей, «Добавить вручную» (дата, длительность `HH:MM`, комментарий, billable).
- В списке обращений — колонка «Время» (опционально, по настройке представления).
- В отчётах — карточка «Затраченное время»: сумма и разбивка по агентам/командам/периоду, отдельно billable.

## 8. Письмо со ссылкой

- Новый шаблон `static/email-templates/portal-magic-link.html`.
- Тема и текст локализуются по `app.lang` (в MVP), позже — по `locale` контакта.
- Отправка через существующий провайдер уведомлений (`internal/notification/providers/email`), чтобы не требовать отдельного SMTP.
- В письме: кнопка «Войти в кабинет», срок действия ссылки, «если вы не запрашивали — проигнорируйте».
- В dev-режиме ссылка дополнительно пишется в лог — удобно для локального теста.

## 9. i18n

- Новые ключи `portal.*` в `en-US.json` и `ru-RU.json` (остальные локали падают на английский).
- Статусы: бейдж по `category` (`portal.status.open/waiting/resolved`), рядом — `status.name` как есть, потому что статусы редактируются админом.
- Время: форматирование `X ч Y мин` через общий `shared-ui/utils/datetime`.
- Ошибки авторизации — из общего пула `auth.*`/`portal.auth.*`.

## 10. Интеграция в сборку

- `frontend/vite.config.js`: третий режим `portal` (`apps/portal`, outDir `dist/portal`, base `/portal/`, dev-порт 8002).
- `frontend/package.json`: `dev:portal`, `build:portal`.
- `Makefile`: добавить `frontend-build-portal` в `frontend-build`. `FRONTEND_DIST := frontend/dist` уже включает `dist/portal`, менять `STATIC` не нужно.
- `cmd/main.go`: `portalDir = "frontend/dist/portal"`.
- `cmd/init.go`: включить `frontend/dist/portal` в локальный FS.
- `cmd/handlers.go`: `GET /portal`, `/portal/assets/{all:*}`, fallback на `index.html` (по образцу `/widget`).
- `cmd/i18n.go` не меняется — пакеты общие.
- `config.sample.toml`: `[portal] enabled`, `session_lifetime`, `magic_link_ttl`, `default_inbox_id`, `jwt_enabled`; `[rate_limit.portal]`.
- Админка: новый раздел «Портал клиентов» — включение, инбокс по умолчанию, JWT-секрет (показывается один раз), срок сессии.
- Права: новая константа в `internal/authz/models`, добавление в список прав админ-роли (`schema.sql` + миграция), экран ролей.

### Карта файлов

```
internal/portal/                 новый пакет: менеджер, queries.sql, models, session, mailer
internal/timetracking/           записи времени, пересчёт сумм, отчёты
internal/migrations/v2.11.0.go   таблицы + колонки
cmd/portal.go                    HTTP-хендлеры портала
cmd/time_tracking.go             HTTP-хендлеры учёта времени
cmd/portal_auth.go               middleware + сессии
cmd/handlers.go                  маршруты
frontend/apps/portal/            SPA
static/email-templates/portal-magic-link.html
i18n/en-US.json, i18n/ru-RU.json  ключи portal.*
```

## 11. Этапы и оценка

| Этап | Состав | Дни |
|---|---|---|
| 1. Каркас | миграция, `internal/portal`, сессии, magic-link, письмо, `/portal` в сборке, экран логина | 3–4 |
| 2. Чтение | список и карточка обращения, статусы, вложения, i18n, тесты доступа | 3–4 |
| 3. Ответы и создание | отправка сообщений, новое обращение в `portal.default_inbox_id`, вложения, правила закрытых обращений, CSRF/лимиты | 3–4 |
| 4. JWT SSO | настройка секрета в админке, `/auth/exchange`, привязка контакта, тесты | 1–2 |
| 5. Время | `conversation_time_entries`, таймер и ручные записи, пересчёт сумм, блок в карточке | 3–4 |
| 6. Отчёты и права | карточка «Затраченное время», роль тимлида, полировка, e2e | 2–3 |
| **Итого** | | **15–21 день** одного разработчика |

MVP можно урезать: этапы 1–2 (только чтение) — 6–8 дней; без JWT и отчётов — около 10–12 дней.
Если удастся переиспользовать #493/#638, этапы 1–3 сокращаются примерно на 3–5 дней: **12–16 дней** суммарно.

## 12. Тесты

- Юнит: скоуп по контакту (A не видит обращение B), фильтрация `private`/`activity`, одноразовость и TTL токена, блокировка контакта, пересчёт сумм времени, запрет двух таймеров.
- Хендлеры: `cmd/portal_test.go` на `testutil.NewDB` (Postgres в Docker).
- E2E: вход через токен, засеянный в БД; список, карточка, ответ, отображение времени.
- Регресс: агентские API и виджет не должны измениться.

## 13. Риски и открытые вопросы

**Риски**

- Email-уникальность контактов регистрозависима (`index_unique_users_on_email_when_no_ext_id_contact` по `email`, не `LOWER(email)`). Для портала ищем по `LOWER(email)`; возможны дубли. Отдельной миграцией можно добавить функциональный уникальный индекс.
- Контакты без email (WhatsApp-only) не смогут войти — нужен понятный экран.
- Доставляемость magic link зависит от настроенного SMTP; в dev — лог.
- Утечка внутренних данных при неаккуратном запросе — закрываем тестами на скоуп и отдельным DTO, не отдаём модели напрямую.
- Забытый таймер — автостоп через 24 ч.

**Подтверждено**

1. Входят только заранее заведённые контакты, регистрации нет.
2. Клиент видит только итог времени.
3. Billable: колонка в схеме есть, в MVP не отображается; вернуться к вопросу после запуска.
4. Ответы и создание обращений — в MVP.
5. JWT SSO — в MVP.
6. Вложения — в MVP.
7. Портал на `/portal`.
8. Чужие записи времени правят админ и тимлид (кастомная роль с `time_tracking:manage` + проверка доступа к обращению).

**Осталось уточнить перед стартом**

- Какой инбокс назначать для новых обращений из портала (`portal.default_inbox_id`)? Рекомендация — email-инбокс поддержки.
- Нужен ли отдельный домен для портала позже (сейчас `/portal`).
- Политика хранения magic-link-токенов: чистить ли просроченные по cron или раз в сутки при запросе.

## 14. Вне MVP

- Счета/акты и биллинг по часам.
- История CSAT и опросы в кабинете.
- Показ SLA-дедлайнов клиенту.
- 2FA для клиентов.
- Кастомный домен портала.
- Полноценная интеграция справочного центра в кабинет (ссылки будут).

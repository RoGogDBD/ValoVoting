# Valorant Poll Overlay

Real-time Twitch Poll overlay for OBS, styled for Valorant streams.  
A Go backend connects to Twitch EventSub WebSocket and pushes vote updates to the overlay via WebSocket — no page reload needed.

Модераторы и стример запускают голосование командой в чате. Оверлей показывает карты с их artwork из игры, сортирует по голосам в реальном времени и автоматически скрывается через 30 секунд после завершения.

---

## Prerequisites

- Go 1.22+
- [Twitch Developer Application](https://dev.twitch.tv/console/apps) (Client ID + Secret)

---

## Получение OAuth-токена

Открой в браузере (подставь свой `CLIENT_ID`):

```
https://id.twitch.tv/oauth2/authorize?client_id=CLIENT_ID&redirect_uri=http://localhost:8080&response_type=token&scope=channel:read:polls+channel:manage:polls+chat:read
```

Необходимые скопы:

| Скоп | Зачем |
|---|---|
| `channel:read:polls` | Получать события опросов через EventSub |
| `channel:manage:polls` | Создавать опросы из чата командой `!mapvote` |
| `chat:read` | Читать сообщения чата для команд |

После авторизации Twitch перенаправит на `http://localhost:8080/#access_token=xxxxxx&...` — скопируй значение `access_token` из адресной строки.

---

## Настройка

```bash
cp .env.example .env
# заполни .env
```

| Переменная | Обязательна | Описание |
|---|---|---|
| `TWITCH_CLIENT_ID` | ✓ | Client ID приложения |
| `TWITCH_CLIENT_SECRET` | ✓ | Client Secret приложения |
| `TWITCH_BROADCASTER_ID` | ✓ | Числовой ID стримера на Twitch |
| `TWITCH_ACCESS_TOKEN` | ✓ | OAuth-токен (все три скопа выше) |
| `TWITCH_CHANNEL` | ✓ | Логин стримера в нижнем регистре (например `rogog_`) |
| `CHAT_COMMAND` | — | Команда запуска голосования (по умолчанию `!mapvote`) |
| `DEFAULT_POLL_DURATION` | — | Длительность опроса в секундах (по умолчанию `60`, диапазон 15–1800) |
| `PORT` | — | HTTP-порт (по умолчанию `8080`) |

Узнать свой `TWITCH_BROADCASTER_ID`:

```bash
curl -s \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Client-Id: YOUR_CLIENT_ID" \
  "https://api.twitch.tv/helix/users?login=YOUR_LOGIN" \
  | jq '.data[0].id'
```

---

## Запуск

```bash
go run ./cmd/server
```

В логах должно появиться:

```
server: listening on :8080
eventsub: connected
eventsub: subscribed to channel.poll.begin
eventsub: subscribed to channel.poll.progress
eventsub: subscribed to channel.poll.end
chat: connected to Twitch IRC, joining #your_channel
```

---

## Добавление в OBS

1. **Sources → Add → Browser Source**
2. URL: `http://localhost:8080/overlay`
3. Width: `1920`, Height: `1080`
4. Включить **"Shutdown source when not visible"**
5. Custom CSS:
   ```css
   body { background: transparent !important; }
   ```

Оверлей появляется в правом нижнем углу с прозрачным фоном.

---

## Как работает

### Автозапуск голосования из чата

Команду `!mapvote` (или любую другую из `CHAT_COMMAND`) могут писать только **стример и модераторы** — остальные игнорируются.

**Синтаксис:**

```
!mapvote [-карта1] [-карта2] [длительность]
```

| Пример | Результат |
|---|---|
| `!mapvote` | Все карты из пула (до 5 случайных — лимит Twitch) |
| `!mapvote 90` | То же, но опрос идёт 90 секунд |
| `!mapvote -bind -icebox` | Убрать Bind и Icebox |
| `!mapvote -bind, icebox 120` | То же через запятую, 120 секунд |
| `!mapvote -bind,icebox,lotus` | Убрать три карты одним флагом |

Исключение карт работает по **префиксу без учёта регистра**: `-as` уберёт Ascent, `-ic` — Icebox.

Если после исключений остаётся больше 5 карт — Twitch не позволяет больше 5 вариантов ответа, поэтому берутся 5 случайных из оставшихся.

**Текущий пул карт:**

```
Abyss · Ascent · Bind · Haven · Icebox · Lotus · Pearl · Split · Sunset
```

Список находится в `internal/twitch/maps.go` — обновляется вручную при изменении пула.

### EventSub

Сервер подписывается на три события:

- `channel.poll.begin` — опрос начался
- `channel.poll.progress` — обновление голосов в реальном времени
- `channel.poll.end` — опрос завершён, выбирается победитель

На каждое событие in-memory состояние обновляется и рассылается всем подключённым оверлеям через WebSocket.

### Определение фазы

Бейдж в оверлее меняется автоматически по ключевым словам в названии опроса:

| Слово в названии | Бейдж |
|---|---|
| `карт`, `map` | `MAP VOTE` |
| `агент`, `agent` | `AGENT VOTE` |
| Всё остальное | `MAP VOTE` (дефолт) |

---

## Оверлей

- **Прозрачный фон** — нет подложки-панели, карты висят поверх стрима
- **Artwork карт** — изображения подгружаются с `valorant-api.com` при старте страницы
- **Зелёная палитра** — акцентный цвет `#00f0a0` (Valorant mint)
- **Сортировка в реальном времени** — карта с наибольшим числом голосов поднимается наверх, анимация через `transform: translateY`
- **Плашка победителя** — показывает карту-победителя с artwork, скрывается через 30 секунд
- **Переподключение** — при разрыве WebSocket автоматически переподключается с экспоненциальным backoff

---

## API

### `GET /api/poll/state`

Текущее состояние опроса из памяти.

```json
{
  "phase": "map",
  "poll_id": "abc123",
  "title": "Выбор карты",
  "status": "active",
  "choices": [
    { "id": "1", "title": "Ascent", "votes": 42 },
    { "id": "2", "title": "Bind",   "votes": 17 }
  ],
  "duration_seconds": 60,
  "started_at": "2025-01-01T12:00:00Z",
  "ends_at": "2025-01-01T12:01:00Z",
  "winner": null
}
```

`phase: "idle"` — нет активного опроса.  
`winner` — заполняется после завершения, содержит объект `{ id, title, votes }`.

### `GET /ws`

WebSocket-эндпоинт для оверлея. Формат сообщений:

```json
{ "type": "poll_update", "data": { /* то же, что /api/poll/state */ } }
```

### `GET /overlay`

Страница оверлея для Browser Source в OBS.

---

## Переподключение и ошибки

- **EventSub**: exponential backoff 1 с → 2 с → 4 с … максимум 30 с
- **Чат IRC**: тот же backoff
- **Overlay WebSocket**: backoff в браузере, без перезагрузки страницы
- **Отзыв токена**: Twitch пришлёт событие `revocation` — ошибка пишется в лог, нужно обновить токен и перезапустить сервер
- **Ошибка создания опроса**: логируется с кодом ответа Twitch (чаще всего `403` означает недостаточно скопов в токене)

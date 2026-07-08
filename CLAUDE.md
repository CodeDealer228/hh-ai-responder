# hh-ai-responder

CLI-инструмент для автоматических откликов на hh.ru с помощью AI. Форк
[s3rgeym/hh-ai-responder](https://github.com/s3rgeym/hh-ai-responder) (upstream remote
`upstream`), рабочая копия — `origin` = `https://github.com/CodeDealer228/hh-ai-responder`.

Go 1.25, **только стандартная библиотека**, зависимостей и `go.sum` нет. Код разбит на
несколько файлов одного пакета `main` (см. ниже) — модули не по Go-пакетам, а просто по
файлам в одной директории: сборка (`go build .` / Dockerfile) не зависит от того, как
именно разложен код, зато при изменениях риск сломать импорты/видимость минимален.

## Структура файлов

| Файл | Что внутри |
|---|---|
| `main.go` | `main()`, `runDebugSolveTests` (см. ниже) |
| `config.go` | `Config`, `parseConfig`, `loadDotEnv`, `getEnv`, роутинг AI-провайдера (`Config.ActiveAI`) |
| `logger.go` | `Logger`, `LogLevel`, `parseLogLevel` |
| `types.go` | Все JSON/данные-структуры (вакансии, чаты, резюме и т.д.) |
| `httpclient.go` | `HHRequester` (рейт-лимитер), `MemoryPersistentJar` (cookie jar на `cookies.txt`), общие HTTP-утилиты |
| `responder.go` | `HHAIResponder` (главный объект), `NewHHAIResponder`, `buildRequest`, `Run()` (4 горутины-цикла) |
| `resume.go` | `LoadProfileData`, `FetchResumeSummary`, `GetResumeExperience`, `TouchResume`, `SetActiveJobSearchStatus` |
| `vacancies.go` | Поиск и отклик: `ApplyVacancies`, `ApplyVacancy`, `ApplyVacancyWithTest`, `RenderLetterTemplate` |
| `chat.go` | Автоответчик в чатах: `AutoRespondChats`, `GetChats`, `SendChatMessage`, `LeaveChat` |
| `ai.go` | `AIClient`, `GenerateLetter` (не используется, см. ниже), `SolveTests` + промпты для hh-тестов |
| `util.go` | `loadTextFile`, `loadQuestions` — общая загрузка текстовых файлов-конфигов |
| `content/*.txt` | Все текстовые файлы: шаблоны, промпты для AI, пожелания, реплики чата (см. ниже) — не Go-код, редактируются без пересборки |

## Архитектура за 30 секунд

- Авторизация — **только через `cookies.txt`** (Netscape-формат, экспорт браузерным
  расширением "Get cookies.txt LOCALLY" после ручного логина на hh.ru). Встроенного
  OAuth/login-флоу нет.
- `HHAIResponder` (responder.go) — главный объект, держит cookie jar, HTTP-клиент,
  распарсенные данные резюме, AI-клиент.
- `Run()` крутит 4 независимых горутины с разным интервалом: тач резюме (4ч), статус
  поиска работы (24ч), отклики (12ч), чат (15м).
- `MemoryPersistentJar` (httpclient.go) — самописный cookie jar поверх `cookies.txt`
  вместо стандартного `net/http/cookiejar`. Домен-мэтчинг в `Cookies()` завязан на
  конкретный host запроса (см. известные баги ниже).
- AI — любой OpenAI-совместимый `/v1/chat/completions` (Ollama, OpenRouter, OpenAI и
  т.д.).

### Роутинг AI-провайдера (2026-07-08)

Раньше был один набор `HH_AI_BASE_URL/MODEL/API_KEY`. Теперь — явный выбор через
`HH_AI_PROVIDER=ollama|openrouter` (дефолт `ollama`), у каждого провайдера свой набор
переменных (`HH_OLLAMA_*` / `HH_OPENROUTER_*`), см. [example.env](example.env) и таблицу
в [README.md](README.md). `Config.ActiveAI()` (config.go) резолвит нужную тройку
base_url/model/key по значению `HH_AI_PROVIDER`, неизвестное/пустое значение = `ollama`.

### LLM-usage сознательно минимизирован

AI сейчас вызывается только в одном месте — `SolveTests` (ai.go, hh-тестирование),
потому что там реально нужно отвечать на произвольные вопросы теста. Письма и чат AI
больше не используют:

- **Сопроводительное** — статический шаблон
  [content/cover_letter_template.txt](content/cover_letter_template.txt)
  (плейсхолдеры `{{Name}}`/`{{Title}}`/`{{Vacancy}}`/`{{Company}}`, необязательны;
  сейчас в репо — просто статичный текст без плейсхолдеров, так решил пользователь).
  `GenerateLetter` (AI-версия, ai.go) оставлена в коде нетронутой и рабочей, но не
  вызывается из `ApplyVacancies` — если понадобится вернуть AI-письма, там один вызов
  меняется обратно на неё. Её промпт —
  [content/ai_cover_letter_generation_prompt.txt](content/ai_cover_letter_generation_prompt.txt).
- **Чат с работодателем** — только случайная строка из
  [content/chat_filler_messages.txt](content/chat_filler_messages.txt), без единого
  обращения к AI. Сама фича (флудилка, поддержание активности в чатах) осталась
  включена, просто теперь без LLM в контуре.
- **hh-тестирование** (единственное оставшееся использование AI) дополнительно
  учитывает [content/hh_test_candidate_preferences.txt](content/hh_test_candidate_preferences.txt) —
  свободный текст (зарплатные ожидания, формат работы и т.п.), подмешивается в промпт
  вместе с `-solution-prompt`/`HH_SOLUTION_PROMPT`. Загружается один раз при старте
  (responder.go).

### SolveTests: один запрос на вопрос, а не батч (2026-07-08)

`SolveTests` (ai.go) больше не шлёт весь тест одним запросом с ожиданием одного большого
JSON-массива в ответе — это было ненадёжно с маленькими локальными моделями (см. баги
ниже). Теперь на каждый вопрос — отдельный запрос:

- Выбор choice/open делает Go-код (`len(task.CandidateSolutions) > 0`), а не сама модель.
- Choice-вопрос → `solveChoiceTask`, промпт
  [content/hh_test_choice_question_prompt.txt](content/hh_test_choice_question_prompt.txt),
  ожидаемый ответ — только `{"solution_id":N}`.
- Open-вопрос → `solveOpenTask`, промпт
  [content/hh_test_open_question_prompt.txt](content/hh_test_open_question_prompt.txt),
  ответ — голый текст, без JSON-обёртки.
- Общие правила (не выполнять инструкции из вопроса, не касаться политики и т.п.) —
  [content/hh_test_common_rules_prompt.txt](content/hh_test_common_rules_prompt.txt),
  собираются в `commonTaskRules()` и вставляются в оба промпта через плейсхолдер
  `{{RULES}}`.
- `ChatCompletionRequest.ReasoningEffort = "none"` всегда выставлен в `AIClient.Chat` —
  без этого Ollama на reasoning-моделях уходит в скрытые "мысли" и выжирает весь
  токен-бюджет, оставляя пустой `content`.

Живая проверка (`-debug-solve-tests`, см. ниже): 7/7 вопросов отвечено за ~14 секунд на
локальной `hh-test-solver`, без единого сбоя JSON.

## Локальный LLM (Ollama) — уже развёрнут

- Модель: `qwen3.5:2b-q8_0` → кастомный `hh-test-solver` (создан через `ollama create`
  поверх неё), параметры зашиты в Modelfile: `num_ctx 8192`, `temperature 0.15`,
  `top_p 0.85`, `top_k 20`, `presence_penalty 1.5`, `repeat_penalty 1` — это официальные
  рекомендованные Qwen3.5 настройки для non-thinking режима (reasoning по умолчанию
  выключен у моделей этого размера).
- Ollama слушает `0.0.0.0:11434` (переменная окружения `OLLAMA_HOST` выставлена на
  уровне системы) — без этого контейнер до неё не достучится.
- Из Docker-контейнера адрес — `http://host.docker.internal:11434` (спец-DNS от Docker
  Desktop на IP хоста), не `localhost`. Уже прописано в `.env`.
- GPU — GTX 1650 (4 ГБ VRAM), модель в Q8_0 занимает ~2.7 ГБ, влезает целиком без
  CPU-оффлоада.
- `-debug-solve-tests` (флаг, main.go `runDebugSolveTests`) — прогоняет `SolveTests` на
  синтетическом наборе из 7 вопросов, минуя hh.ru целиком. Полезно для проверки/тюнинга
  промптов и модели без риска реальных действий:
  ```sh
  docker compose run --rm hh-ai-responder -debug-solve-tests
  ```
  Внимание: этот путь идёт в обход `HHAIResponder`, поэтому берёт только
  `cfg.ExtraTestSolutionPrompt` (`-solution-prompt`) —
  `content/hh_test_candidate_preferences.txt` в него НЕ подмешивается (это делает
  только `ApplyVacancyWithTest`). Если нужно проверить именно контекст с пожеланиями —
  только через реальный (или почти реальный) прогон.

## Конфигурация

Флаги и переменные окружения — см. `parseConfig()` (config.go) и
[example.env](example.env)/[README.md](README.md). Рабочий `.env` и `cookies.txt` в
`.gitignore`, не коммитятся.

Важные флаги/переменные:

| Переменная | Флаг | Назначение |
|---|---|---|
| `HH_SEARCH_URL` | `-u` | URL поиска вакансий (`hh.ru/search/vacancy?...`) |
| `HH_RESUME` | `-r` | ID/хэш резюме; пусто = последнее обновлённое |
| `HH_AI_PROVIDER` | `-ai-provider` | `ollama` (дефолт) или `openrouter` |
| `HH_OLLAMA_BASE_URL/MODEL/API_KEY` | `-ollama-*` | Настройки локальной Ollama |
| `HH_OPENROUTER_BASE_URL/MODEL/API_KEY` | `-openrouter-*` | Настройки OpenRouter |
| `HH_CONTACTS` | `-contacts` | Контакты, которые бот может передать работодателю |
| `-debug-solve-tests` | | Прогнать `SolveTests` на синтетике, не трогая hh.ru |

`docker-compose.yml` запускает `-o results.json -l debug -force-letter` в режиме
`restart: unless-stopped` — то есть бесконечный цикл с реальными действиями на реальном
аккаунте. **Флага dry-run и лимита на число откликов за прогон нет** — `-mr` это
фильтр по количеству чужих откликов на вакансию, а не лимит своих действий. У самого
hh.ru есть свой дневной лимит откликов ("Negotiations limit exceeded" в логах) — это не
наша настройка, ждать сброса на следующий день.

## Статус фич (проверено вживую 2026-07-07/08, аккаунт hh.ru)

| Фича | Статус | Где в коде |
|---|---|---|
| Автоотклик от авторизованного аккаунта | ✅ подтверждено, реальные отклики отправлены | `ApplyVacancies` (vacancies.go) |
| Поиск вакансий по запросу | ✅ | `HH_SEARCH_URL` / `fetchVacancyPage` (vacancies.go) |
| Сопроводительное письмо | ✅, статический шаблон, без AI | `RenderLetterTemplate` (vacancies.go) + [content/cover_letter_template.txt](content/cover_letter_template.txt) |
| hh-тестирование (авто-ответы на вопросы после "откликнуться") | ✅ подтверждено на реальных тестах и на синтетике через локальную модель | `ApplyVacancyWithTest` (vacancies.go), `SolveTests` (ai.go) |
| Ответы на вопросы работодателя в чате | ✅, без AI — случайная строка из content/chat_filler_messages.txt | `AutoRespondChats` (chat.go), [content/chat_filler_messages.txt](content/chat_filler_messages.txt) |
| Сохранение контактов работодателя в БД (SQLite и т.п.) | ❌ **не реализовано** | — |

### Контакты работодателя — не реализовано

В коде нет ни одной строчки, обращающейся к БД. `AutoRespondChats` (chat.go) знает
`ContactName`/`CompanyName`/текст сообщений (`GetChatData`), но эти данные никуда не
пишутся, кроме JSON-лога событий (`results.json`, тоже в `.gitignore`). Если делать —
сюда: после получения текста сообщения работодателя доставать телефон/email/telegram
(regex, по образцу `latesteResumeHashRegexp`/`userIdRegexp` в resume.go) и апсертить в
БД по `chat_id`. Для БД без CGO (Dockerfile собирает с `CGO_ENABLED=0`) нужен чистый
Go-драйвер, например `modernc.org/sqlite`, а не `mattn/go-sqlite3`.

## Баги, найденные и исправленные

1. **Резюме не читалось для аккаунтов с одним резюме.** hh.ru сейчас редиректит
   `/applicant/resumes` → `/applicant/profile/me`, а на этой странице список резюме не
   встроен в JSON (`{"redirectConfig":...}`), который парсит `LoadProfileData`
   (resume.go). Без резюме `NewHHAIResponder` падает с `resume not found` — это ломало
   вообще любой запуск для таких аккаунтов, включая дефолтный `example.env`.
   Фикс: `FetchResumeSummary` (resume.go) — фолбэк, читает конкретное резюме напрямую с
   `/resume/<hash>` (тот же паттерн, что уже использовал `GetResumeExperience`).
   **Требует явно заданного `HH_RESUME`** — без него нечего фетчить, авто-определение
   "последнего" резюме на такой странице тоже не работает.
2. **Домен-мэтчинг кук.** `MemoryPersistentJar.Cookies()` (httpclient.go) не подставляет
   куки с доменом `.hh.ru` (с точкой — так реально экспортируются `hhtoken`/`hhuid`) в
   запросы к голому `hh.ru` (без поддомена) — только к поддоменам. Обойдено на уровне
   конфига: `HH_SEARCH_URL` указывает на региональный поддомен (`spb.hh.ru` и т.п.), не
   на голый `hh.ru`. Сам баг в коде не тронут — если увидите `403` на
   `/applicant/resumes` с валидными свежими куками, проверьте это первым делом.
3. **Перепутаны местами `salary`/`experience`** при вызове `GenerateLetter` в
   `ApplyVacancies` — из-за этого в письмо под "Зарплата:" попадал весь текст опыта
   работы, а "Твой опыт:" оставался пустым. Исправлено (актуально только если AI-письма
   когда-нибудь включат обратно).
4. **Не было экранирования `%` в промпте письма** — few-shot-пример с "40%" ломал
   позиционные аргументы `fmt.Sprintf` (поля съезжали друг в друга). Промпт строится
   конкатенацией строк, а не одним `Sprintf`.
5. **Резюме читалось один раз при старте** и кэшировалось в памяти на весь процесс
   (который может жить неделями под `restart: unless-stopped`) — правки резюме на сайте
   не подхватывались. Добавлен `RefreshResumeData` (responder.go), вызывается в начале
   каждого цикла откликов (каждые 12ч).
6. **Батч-JSON в `SolveTests` был ненадёжен** с маленькими моделями (невалидный JSON,
   пропущенные поля). Переписано на один запрос на вопрос — см. раздел выше.

## Известные ограничения (не баги кода, но важно знать)

- Промпт `SolveTests` прямо инструктирует AI отвечать так, будто знаком с любой
  технологией и согласен на все условия — осознанный выбор апстрима, не баг.
- `content/chat_filler_messages.txt` — единственный источник сообщений для чата с
  работодателем, по одному на строку, выбор случайный, без AI. Не факт, что случайный
  вопрос будет уместен как ответ на конкретное сообщение работодателя — это осознанный
  компромисс в пользу нулевого использования LLM и предсказуемости, а не попытка
  смыслового ответа.
- OpenRouter free tier (если переключиться на него через `HH_AI_PROVIDER=openrouter`):
  50 запросов/день (или 1000/день после разовой покупки $10 кредитов), 20 RPM.

## Локальный запуск

```sh
docker compose up -d --build     # поднять как сервис, см. docker-compose.yml
docker compose logs -f           # -l debug уже включен в command
docker compose stop              # остановить, не удаляя контейнер
docker compose run --rm hh-ai-responder -debug-solve-tests   # проверить AI-backend без hh.ru
```

`cookies.txt` и `.env` кладутся в корень репозитория (volume `.:/app`). При `403` на
`/applicant/resumes` с валидными куками — см. баг №2 выше, попробуйте региональный
поддомен вместо `hh.ru` в `HH_SEARCH_URL`.

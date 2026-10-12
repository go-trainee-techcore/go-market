# Запуск проекта: чек-лист ментора

Что сделать, чтобы стажёры могли начать работу, и что подготовить к
отсутствию. Шаги идут по порядку: каждый следующий опирается на предыдущий.

Переменная для команд ниже:

```bash
R=go-trainee-techcore/go-market
```

## Уже сделано

Проверено 12 октября:

- организация `go-trainee-techcore`, публичная репа `go-market`, `develop` —
  ветка по умолчанию;
- rulesets: `develop` — только squash и линейная история, `main` — только merge;
  на обеих ветках ревью от code owners и обход для роли admin в режиме «только
  через PR»;
- классическая защита веток удалена;
- secret scanning, push protection и Dependabot alerts включены;
- в скелете: модуль `github.com/go-trainee-techcore/go-market`, тесты на весь
  `shared/`, линтер — 0 замечаний.

## Этап 1. До переноса скелета

**1.1. Команда `mentors`.** На неё ссылается CODEOWNERS. Если команды нет, строки
с ней GitHub считает недействительными, и пути ментора останутся без защиты.

```bash
gh api -X POST orgs/go-trainee-techcore/teams -f name=mentors -f privacy=closed
gh api -X PUT orgs/go-trainee-techcore/teams/mentors/memberships/futod4m4 -f role=maintainer
gh api -X PUT orgs/go-trainee-techcore/teams/mentors/repos/$R -f permission=push
```

`privacy=closed` — это видимая команда; секретные в CODEOWNERS не работают.
Право записи команде нужно выдать явно, даже если у участников оно уже есть.

**1.2. Ревью-бот на твоей подписке.** Бот работает по запросу — когда на PR
вешают метку `claude-review`, — и на Sonnet, чтобы меньше тратить лимит:

```bash
claude setup-token                                  # долгоживущий токен подписки
gh secret set CLAUDE_CODE_OAUTH_TOKEN -R $R
gh label create claude-review -R $R --color 7057FF --description "Запросить ревью от Claude"
```

Лимит подписки общий с твоей собственной работой в Claude и Claude Code.
Расход видно в claude.ai → Settings → Usage; если перестанет хватать — usage
credits, Max или API-ключ (`ANTHROPIC_API_KEY` и `anthropic_api_key` в workflow).

Токен — это доступ к твоей подписке. Любой с правом записи технически может
вытащить секрет через workflow в своей ветке, поэтому, если кто-то уйдёт из
команды, перевыпусти токен.

**1.3. Настройки репы.**

```bash
gh api -X PATCH repos/$R -F delete_branch_on_merge=true     # удалять ветку после мержа
gh api -X PUT repos/$R/automated-security-fixes              # Dependabot сам откроет PR с исправлением
gh label create needs-mentor -R $R --color D93F0B --description "Нужен ответ ментора"
```

**1.4. Go на своей машине.** Сейчас в системе Go 1.25.7, а 1.26 подкачивается
автоматически — при тестах сыплются `version does not match go tool version`.
Поставь 1.26 полностью (`brew install go` или пакет с go.dev) и проверь, что
`task test` проходит без этих строк.

**1.5. `.gitleaks.toml` — твоя задача.** Стандартный набор правил gitleaks пароль
в строке подключения не ловит (проверено). Положи в корень скелета:

```toml
[extend]
useDefault = true

[[rules]]
id = "url-with-credentials"
description = "Логин и пароль прямо в URL подключения"
regex = '''[a-zA-Z][a-zA-Z0-9+.-]*://[^\s/:@'"]+:[^\s/:@'"]+@[^\s/'"]+'''

[allowlist]
description = "Учебные креды локальной инфраструктуры из docker-compose"
regexes = ['''gomarket:gomarket@localhost''']
```

gitleaks подхватывает файл из корня сам, `secrets.yml` менять не нужно.

## Этап 2. Перенос скелета в go-market

Локальная папка `gomarket` — черновик со своей историей. Каноничной становится
`go-market` на GitHub; после переноса черновик можно удалить.

```bash
cd ~/Desktop/internship
git clone git@github.com:go-trainee-techcore/go-market.git
cd go-market                          # клонируется develop — она по умолчанию
git switch -c chore/skeleton

rsync -a --exclude .git --exclude coverage.out ../gomarket/ ./
rm .github/workflows/auto-assign.yml .github/workflows/proof-html.yml

git add -A
git status                            # проверь: нет .env, бинарников, мусора
git commit -m "chore: каркас монорепо"
git push -u origin chore/skeleton
gh pr create --base develop --title "chore: каркас монорепо" --fill
```

Шаблонные workflow удаляются, потому что `auto-assign` назначал бы тебя на
каждый PR стажёров, а `proof-html` проверял бы HTML, которого в репе нет.

На этом PR впервые запустится CI. Дождись, что «Линтер», «Тесты» и «gitleaks»
зелёные. PR трогает только твои пути, апрувить его некому — мержи через обход:
галка *Merge without waiting for requirements to be met*.

Затем **релиз в `main`**, чтобы CODEOWNERS и workflow появились и там:

```bash
gh pr create --base main --head develop --title "chore: каркас монорепо" --fill
```

Этот первый релиз тоже мержишь через обход. Все следующие открывает кто-то из
стажёров, а ты апрувишь.

## Этап 3. После первого прогона CI

**3.1. Обязательные проверки.** Settings → Rules → каждый ruleset → *Require
status checks to pass* → *Add checks*: `Линтер`, `Тесты`, `gitleaks`. Найти их
можно, только когда они хотя бы раз отработали — поэтому этот шаг после этапа 2.

Не добавляй `Контракты` и `Образ ...`: они запускаются по условию, и пропущенная
обязательная проверка вешает PR навсегда.

**3.2. CodeQL.** Settings → Code security → Code scanning → CodeQL analysis →
*Set up* → *Default*. Проверь, что в языках появился Go; если нет — добавь
вручную.

**3.3. Правило по результатам CodeQL** — только после первого прогона CodeQL на
каком-нибудь PR. Rulesets → *Require code scanning results*: tool CodeQL,
security alerts *High or higher*, alerts *Errors*.

**3.4. Проверка схемы целиком.** Попроси одного из стажёров открыть
пустяковый PR, который трогает `shared/` (например, опечатку в комментарии), и
проверь:

- GitHub сам запросил ревью у команды `mentors`;
- бот оставил ревью;
- без апрува и без зелёных проверок кнопка мержа недоступна;
- твой апрув снимает блокировку.

## Этап 4. Подготовка к старту стажёров

**4.1. Владение сервисами — решено.** Максим: Identity и Payment, Лера: Cart и
Catalog, Order и сага — вместе. Вписано в [`docs/onboarding.md`](../onboarding.md),
раздел 2, исполнители в бэклоге назначены.

Почему так: Payment — самый рискованный сервис, а Identity — критический путь
второго этапа; оба отданы тому, кто быстрее всех закрывает замечания ревью и
пишет тесты. Лера начинает со своего Cart на основе протестированного кода
Максима, а Catalog у неё на третьем этапе, когда она успеет разогнаться. Если
Максима первым заберут на проект, Лере достанутся два самых сложных сервиса —
это осознанный риск.

**4.2. Доска.** GitHub Projects в организации, колонки *Todo / In progress /
Review / Done*. Задачи на первые две недели пишешь ты, дальше они:

| Неделя | Задача |
|---|---|
| 1 | окружение: прочитать онбординг, поднять инфраструктуру, запустить example, `task check` зелёный |
| 1 | контракты gRPC: `ValidateToken` (Identity), `Reserve` / `ConfirmReservation` / `ReleaseReservation` (Catalog), `GetCartSnapshot` (Cart), `Charge` / `Refund` (Payment) |
| 1 | схемы событий Kafka: все шесть из онбординга |
| 1 | OpenAPI публичных ручек и таблица кодов ошибок по сервисам |
| 2 | Cart: слияние двух Cart API в `services/cart` по эталону |
| 2 | Identity: регистрация и логин |

**4.3. Предупредить Максима**, что конфиг в монорепо — из env через
`go-envconfig`, а не viper с yaml, как у него в Cart API. Переход минут на
десять, но лучше знать заранее.

**4.4. Кикофф, 1–1,5 часа:** роли и владение, скоуп и план по неделям, процесс
PR (раздел 6 онбординга), правила про AI, проход по `services/example` и
`shared/`, первые задачи с доски.

## Этап 5. Перед отъездом

Задача — чтобы на 18 дней вместо тебя требовалось как можно меньше, а
замещающему досталась работа, которую он реально сделает.

**5.1. Поставь отъезд на удачные недели.** Хуже всего — первая неделя (контракты:
ты нужен постоянно) и недели 5–8 (Order и сага живут в твоих путях, каждый PR
ждёт ментора). Лучше всего — недели 2–5: стажёры пишут свои сервисы, и почти
ничего не требует твоего апрува. Если отъезд выпадает на сагу — переставь
порядок: на время отсутствия возьмите наблюдаемость или доделку сервисов, а
сагу — после.

**5.2. Контракты смержены до отъезда.** Изменение `.proto` требует апрува ментора;
пусть за 18 дней его не понадобится.

**5.3. Доска нарезана на всё отсутствие плюс неделю**, у каждой задачи — критерий
готовности. Замещающему не нужно придумывать работу.

**5.4. Замещающий в команде `mentors`:**

```bash
gh api -X PUT orgs/go-trainee-techcore/teams/mentors/memberships/<логин> -f role=member
```

Проверь, как в шаге 3.4: тестовый PR в `shared/` должен запросить его ревью, и
его апрув должен снимать блокировку. Админом его делать не нужно.

**5.5. Передача, 30 минут:** замещающий и оба стажёра, проход по
[`substitute.md`](substitute.md). Там всё, что от него нужно, — три пункта.

**5.6. Еженедельный статус письменно.** Закреплённый issue «Статус», куда
стажёры по четвергам пишут: что сделано, что дальше, где застряли. Сможешь —
прочитаешь, не сможешь — вернувшись, увидишь всю картину за 18 дней.

**5.7. После возвращения:** прочитать новые ADR, сделать один глубокий проход по
всему коду, убрать замещающего из команды:

```bash
gh api -X DELETE orgs/go-trainee-techcore/teams/mentors/memberships/<логин>
```

## Этап 6. Позже, не блокирует старт

- **Gateway:** конфиг NGINX с `auth_request` в Identity — к неделям 2–3, когда
  появится проверка токена;
- **Mac mini:** проверить диск (`lsblk -d -o NAME,ROTA,SIZE,MODEL`, `ROTA=1` —
  нужен SSD), Tailscale, k3s, Helm с тремя неймспейсами — к неделе 8;
- **Grafana:** дашборд и алерты на долю ошибок — запросы есть в
  [`shared/README.md`](../../shared/README.md#запросы-которые-пригодятся);
- **приёмочные e2e** по шести пунктам MVP и toxiproxy для хаос-тестов.

## Что в скелете ещё не проверено

- **Версии GitHub Actions** (`golangci-lint-action@v8`, `buf-action@v1`,
  `claude-code-action@v1`) — подтвердятся первым прогоном CI в этапе 2;
- **gRPC, Kafka, Redis в `shared/`** пока нет — список с ориентировочными
  сроками в [`shared/README.md`](../../shared/README.md#чего-в-shared-пока-нет);
- **порог покрытия** считается только по пакетам `internal/service`; на
  `shared/` тесты есть, но в порог они не входят.

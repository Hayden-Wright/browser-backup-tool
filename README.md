# BrowserBackup

**Портативная утилита для Windows: сохраняет и восстанавливает профили браузеров.**
Один `.exe`, без установки. Ничего не пишет в систему — ни в реестр, ни в `%APPDATA%`.

<p align="center">
  <img src="docs/screenshot.png" alt="Окно BrowserBackup" width="700">
</p>

---

## Скачать

**[⬇️ Последний релиз](https://github.com/Hayden-Wright/browser-backup-tool/releases/latest)** → `BrowserBackup.exe`

Запускать двойным щелчком. Установка не требуется — можно положить на флешку.
При первом запуске SmartScreen может предупредить о «неизвестном издателе»
(файл без платной подписи) — нажмите **«Подробнее» → «Выполнить в любом случае»**.

> **Требование:** Windows 10/11 (64-бит). Нужен **WebView2 Runtime** — в Windows 11
> и большинстве сборок Windows 10 он уже установлен. Если окно не открывается,
> поставьте [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/).

---

## Возможности

- 🔒 **Полностью портативно** — настройки рядом с `.exe`, временные данные движка
  удаляются при выходе, в профиле пользователя и реестре не остаётся следов.
- 🧩 **Много браузеров** — Firefox- и Chromium-семейства определяются автоматически,
  плюс режим «Другой браузер» для любого.
- ✅ **Проверка целостности** — SHA-256 считается на лету при копировании, битый
  бэкап сразу видно, а не «тихо сломанный».
- 🛟 **Безопасное восстановление** — текущий профиль не удаляется: сначала делается
  страховочная копия (`_safety`), откатиться можно в любой момент.
- ⚡ **Отзывчивый интерфейс** — долгие операции идут в фоне, окно не «зависает».
- 🌐 **Русский / English**, тёмная тема.

---

## Поддерживаемые браузеры

| Семейство | Браузеры | Пароли |
|---|---|---|
| **Firefox** | Zen, Firefox, LibreWolf, Waterfox, Floorp | ✅ переносятся файлами |
| **Chromium** | Chrome, Edge, Brave, Vivaldi, Chromium, Thorium, Ungoogled Chromium, SRWare Iron, Arc | ⚠️ только через синхронизацию аккаунта |
| **Другой** | любой — указать папку профиля вручную | зависит от браузера |

### Про пароли — это важно

- **Firefox-семейство** хранит пароли и входы в профиле — после восстановления
  вы будете залогинены везде, как раньше.
- **Chromium-семейство** шифрует пароли и куки ключом вашей учётной записи Windows.
  После переустановки Windows ключ меняется, поэтому старые пароли и куки
  **не расшифруются**, даже если файлы скопированы идеально. Файлами переносятся
  закладки, история, расширения и настройки; входы и пароли возвращаются **только
  через вход в аккаунт (Google/Microsoft/Brave/…)** и включённую синхронизацию.

Программа предупреждает об этом прямо в окне, рядом с каждым таким браузером.

---

## Как пользоваться

**Бэкап**
1. Запустите `BrowserBackup.exe`.
2. В разделе «Браузеры и профили» нажмите на браузер — раскроется список профилей,
   отметьте нужные галочками.
   - «активный» — профиль, которым вы пользуетесь сейчас;
   - «пустой» — служебная папка без данных (бэкапить не нужно). У Zen рядом с рабочим
     профилем может лежать пустая заготовка «Default Profile» — это **не дубликат**.
3. Укажите «Папку для бэкапа» кнопкой «Изменить…».
   > ⚠️ Не храните бэкап на диске `C:` — при переустановке Windows он сотрётся.
   > Программа об этом предупредит.
4. Нажмите «Создать бэкап». **Браузер должен быть закрыт** — иначе часть файлов
   не скопируется, и программа не даст продолжить.

**Восстановление**
1. Установите браузер(ы), запустите каждый **один раз** и закройте — чтобы браузер
   сам создал свежую папку профиля.
2. Запустите `BrowserBackup.exe`, укажите ту же папку бэкапа.
3. В разделе «Восстановить из бэкапа» отметьте нужное и нажмите «Восстановить».

---

## Особенности

- Профиль переносится в папку, которую создал **новый** браузер, а не в папку со
  старым случайным id — так работает корректно.
- **Кэш не копируется** — он не нужен для переноса данных и занимает много места.
- Каждый бэкап — отдельная папка с датой и временем в имени
  (`zen_Default (release)_2026-10-09_18-41-12`). Новый бэкап **никогда** не
  перезаписывает старый.
- Внутри бэкапа — `manifest.json` и `ПРОЧТИ_МЕНЯ.txt`: браузер, версия, профиль,
  дата, Windows, компьютер, число файлов, размер, контрольные суммы.
- Все действия пишутся в `browser-backup.log` в папке бэкапа. Кнопка
  «Открыть файл лога» открывает сам журнал, «Открыть папку» — папку в проводнике.
- Понятные сообщения об ошибках вместо кодов.

---

## Портативность и настройки

Программа **не оставляет следов** в системе:

| Что | Где |
|---|---|
| Настройки (папка бэкапа, язык, «Другой браузер») | `BrowserBackup-settings.json` **рядом с `.exe`** |
| Временные данные WebView2 (~десятки МБ) | во временной папке Windows, **удаляются при выходе** |
| Журнал | `browser-backup.log` в папке бэкапа |
| Реестр / `%APPDATA%` | **не используется** |

Папку с `.exe` можно целиком перенести на флешку — настройки переедут вместе с ней.
Удалите `BrowserBackup-settings.json` — настройки сбросятся к значениям по умолчанию.

**Свой список браузеров.** Положите рядом с `.exe` файл `browsers.json` — он будет
использован вместо встроенного списка:

```json
{
  "browsers": [
    { "key": "mybrowser", "name": "My Browser", "family": "firefox",
      "proc": ["mybrowser"], "roaming": "MyBrowser", "local": "MyBrowser" },
    { "key": "other", "name": "Other", "family": "chromium",
      "proc": ["other"], "local": "Other/User Data" }
  ]
}
```

`family`: `"firefox"` (профили в `%APPDATA%`, пароли файлами) или `"chromium"`
(профили в `%LOCALAPPDATA%`, пароли только аккаунтом). `roaming` / `local` /
`packages` — пути-кандидаты (можно несколько через `|`). Если файла нет или он
повреждён — используется встроенный список.

---

## 🔐 Совет по безопасности

Для Firefox-семейства бэкап содержит **пароли и активные сессии** — это фактически
ключи от ваших аккаунтов. Не выкладывайте папку бэкапа в облако и мессенджеры.
Для надёжности держите её на зашифрованном диске (BitLocker / VeraCrypt).

---

## Сборка из исходников

Нужен [Go](https://go.dev/dl/) 1.26+. Кросс-компиляция под Windows (в т.ч. из WSL):

```sh
GOOS=windows GOARCH=amd64 GOFLAGS=-mod=mod \
  go build -ldflags "-s -w -H windowsgui" -o BrowserBackup.exe .
```

> Флаг `-H windowsgui` обязателен — иначе при запуске открывается лишнее окно консоли.

Тесты логики (хеши, манифест, настройки) гоняются под Linux:

```sh
GOOS=linux go test .
```

Подробности об устройстве и истории проекта — в [`HANDOFF.md`](HANDOFF.md).

---

## Технологии

Go · WebView2 (встроенный движок) · свой COM-диалог выбора папки
(`IFileOpenDialog`) · SHA-256 на лету при копировании · без внешних зависимостей
кроме `go-webview2`, `go-ole`, `golang.org/x/sys`.

## Лицензия

Проект распространяется «как есть», без гарантий. Используйте на свой риск —
всегда проверяйте бэкап перед восстановлением.

---

<details>
<summary><b>English (short)</b></summary>

**BrowserBackup** — a single portable Windows `.exe` that backs up and restores
browser profiles after reinstalling Windows. No installation, no traces in the
system (nothing in the registry or `%APPDATA%`).

- **Download:** [latest release](https://github.com/Hayden-Wright/browser-backup-tool/releases/latest) → `BrowserBackup.exe`.
- **Requires:** Windows 10/11 x64 + WebView2 Runtime (preinstalled on Windows 11).
- **Firefox family** (Zen, Firefox, LibreWolf, Waterfox, Floorp): passwords are
  stored in the profile and **transfer with the files**.
- **Chromium family** (Chrome, Edge, Brave, Vivaldi, …): passwords and cookies are
  encrypted with your Windows account key, so they **will not decrypt** after a
  Windows reinstall — sign-ins come back only via account sync. Bookmarks,
  history, extensions and settings do transfer.
- Backup: expand a browser → pick profiles → choose a folder (**not on `C:`**) →
  Create backup. The browser must be closed. Integrity (SHA-256) is checked on the fly.
- Restore: install the browser, run it once and close it, then Restore. A safety
  copy of the current profile is made first; nothing is deleted.
- Settings live in `BrowserBackup-settings.json` next to the `.exe`; carry the
  folder anywhere and they travel with it.

</details>


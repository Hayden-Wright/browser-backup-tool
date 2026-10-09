# ПЕРЕДАЧА РАБОТЫ — BrowserBackup

> Файл для продолжения работы в новом чате. Прочитай его целиком перед началом.
> Дата: 2026-10-09. Последнее состояние кода: см. раздел 2 (свежее — 2а).
>
> **Кратко, где мы:** все баги (падение диалога, «Создать бэкап» молча, галочки
> на бэкапах, «бесконечная проверка целостности», непортативность) — ИСПРАВЛЕНЫ
> и собраны. Осталось: **проверить в GUI руками** (диалоги «Изменить…»/«Добавить
> папку…», «Открыть папку», бэкап большого профиля Zen 2.2 ГБ, restore) — см.
> раздел 4.

---

## 1. Что это за проект (кратко)

`/путь/к/browser-backup-tool` — портативная утилита для Windows (один .exe)
для сохранения/восстановления профилей браузеров. Go + GUI на **WebView2**
(встроенный `ui/index.html`, тёмно-синяя тема, RU/EN). НЕ lxn/walk.
Это **НЕ git-репозиторий** (git status падает — это нормально).

Сборка (кросс-компиляция из WSL), ОБЯЗАТЕЛЬНО с `-H windowsgui`:

```sh
export PATH=$PATH:/путь/к/go-toolchain/go/bin GOPATH=/путь/к/go-toolchain/gopath
cd /путь/к/browser-backup-tool
GOOS=windows GOARCH=amd64 GOFLAGS=-mod=mod go build -ldflags "-s -w -H windowsgui" -o BrowserBackup.exe .
```

Развёрнутые копии: `/путь/к/BrowserBackup/` и
`/путь/к/Downloads/#Test/` (exe + README.txt).

Портативность (см. 2а): настройки — `BrowserBackup-settings.json` РЯДОМ С .EXE;
журнал — `<backup_dir>\browser-backup.log`; данные WebView2 — во временной папке
(`%TEMP%\BrowserBackup-webview-*`), удаляются при выходе. В `%APPDATA%` и реестр
программа НЕ пишет.

---

## 2. Что СДЕЛАНО в этой сессии (правки уже в файлах)

### 2а. Сессия «галочки + портативность» (последняя)

Правки в `ui/index.html`, `settings.go`, `util.go`, `gui.go`, `README.txt`.
Плюс тест `settings_test.go`.

1. **Галочки на бэкапах больше не проставляются сами.** В `render()` бэкапы
   рендерились с классом `sel` (`<div class="bk sel" ...>`) — все сразу
   выглядели выбранными. Убрано: теперь `<div class="bk" ...>`, как у профилей
   («по умолчанию НИЧЕГО не выбрано»). `ui/index.html` ~строка 361.

2. **Полная портативность — программа не оставляет следов вне бэкапов.**
   - **Настройки → рядом с .exe.** Было `%APPDATA%\BrowserBackup\settings.json`,
     стало `BrowserBackup-settings.json` в папке .exe (`settings.go`:
     `settingsPath()` = `filepath.Join(exeDir(), ...)`). Функция `settingsDir()`
     удалена; `exeDir()` берётся из `util.go` (не дублируется). `saveSettings`
     больше не делает `MkdirAll` (папка exe существует) и не падает, если она
     только для чтения — просто пишет в лог.
   - **Данные WebView2 → во временную папку.** Раньше go-webview2 создавал
     `%APPDATA%\<имя exe>\EBWebView` (**замерено: 70 МБ!**). Теперь в
     `runApp()` задаётся `DataPath: os.MkdirTemp("", "BrowserBackup-webview-")`,
     а `defer removeWebViewData(dir)` удаляет папку при выходе (с ретраями —
     WebView2 держит файлы пару секунд). Плюс `cleanupStaleWebViewData()` на
     старте подчищает папки от аварийно завершённых запусков (не трогает
     свежие < 1 мин — чтобы не задеть параллельный экземпляр).
   - **Реестр не используется вообще** (проверено: пишем только `DwmSetWindowAttribute`
     для тёмного заголовка — это в памяти окна, не в реестр).
   - **Проверено на реальной Windows:** запуск из `C:\путь\к\bb-portable-test\`
     → в `%APPDATA%` ничего, данные в `%TEMP%\BrowserBackup-webview-*`,
     при закрытии папка удалена. Настройки при простом запуске/закрытии не
     пишутся (только при смене папки/языка).

### 2б. Сессия «проверка целостности»

Правки в `copy.go`, `integrity.go`, `backup.go`, `restore.go`, `gui.go`,
`ui/index.html`, `README.txt`. Плюс тесты `copy_test.go`, `backup_test.go`.

1. **Один проход хеширования.** `copyFile` теперь считает SHA-256 записанного
   содержимого прямо во время копирования и возвращает хеш. `copyTree` собирает
   `SourceDigest` на лету. Общий хелпер `hashLines([]string)` в `copy.go` (его же
   использует `digestTree`). `digestTree`/`hashFile` остались — нужны для
   проверки СТАРЫХ бэкапов и восстановления.
2. **Backup/restore — вне потока окна.** `jsBackup`/`jsRestore` в `gui.go` теперь
   только запускают горутину и сразу возвращают `{}`. Итог доставляется в JS
   вызовом `window.__opDone(...)`. Прогресс троттлится до ~10 Гц (`lastProg`).
   В JS — `runOp(kind, start)` возвращает промис, который резолвится из
   `__opDone`; если Go сразу ответил `{"error":"busy"}` — резолвим сразу.
3. **Нет повторной проверки после создания.** Сводки собраны на лету →
   `doBackup` помечает манифест `verified:true` без второго прохода.
   `doRestore` проверяет целостность только если `Verified` не выставлен.
4. **`gui.go` получил build-тег `//go:build windows`** — иначе пакет не
   собирался под linux и нельзя было гонять тесты логики (`go test`).
5. **Проверено:** `go test` (4 теста), `go vet`, `gofmt` — чисто; windows-сборка
   ок; на реальной машине `-autojs` прогнал полный путь doBackup (15.8 МБ,
   511 файлов) — окно живо, прогресс идёт, `verified:true`, сводки совпали с
   независимым пересчётом.

### 2в. Сессия «nargs-баг, dialog.go» (первая)

Исходная жалоба пользователя: кнопки «Изменить…», «Добавить папку» — программа
просто закрывается; «Открыть папку» не работает; «Создать бэкап» — ничего не
происходит. Плюс «проверка целостности бесконечная».

### Баг №1 (ГЛАВНЫЙ) — падение при клике на «Изменить…»/«Добавить папку…»

**Причина:** библиотека `github.com/harry1453/go-common-file-dialog v1.2.0`
вызывает COM-методы через `syscall.Syscall(fn, 1, this, arg, 0)` — указывает
число аргументов **1**, хотя аргументов **два** (`this` + параметр).
На старых Go этот счётчик на amd64 игнорировался. В Go 1.17+ рантайм
(`syscall_syscalln` → `asmstdcall`) стал его соблюдать: при `n=1` на стек
кладётся только `this`, второй аргумент теряется, COM-метод читает мусор в RDX
и процесс падает с **0xc0000005**. Сломана почти вся библиотека (`Show`,
`Release`, `SetFolder`, `GetOptions`, `SetTitle`…).

Доказано экспериментально: `syscall.Syscall(SetFolder, 1, obj, item, 0)` →
падение; с `2` → успех.

**Исправление:** `dialog.go` **полностью переписан** — собственная реализация
диалога `IFileOpenDialog` через COM, без библиотеки. Все вызовы идут через
`comCall()` = `syscall.SyscallN(fn, args...)`, где число аргументов берётся из
самого вызова — ошибиться нельзя. Структура vtable описана вручную
(`iFileDialogVtbl` и т.д.). Публичный API (`jsPickFolder`, `jsPollFolder`,
`showFolderDialog`, тип `folderPick`) сохранён — остальной код не менялся.

### Баг №2 — «Создать бэкап» молча ничего не делал

**Причина:** `checkRunning` возвращал `{"running":null}` (Go: `var running []string`
= nil → JSON `null`), а JS делал `check.running.length` → `TypeError: Cannot
read properties of null` → промис отклонялся, `doBackup` молча обрывался.

**Исправление (два уровня, оба внесены):**
- `gui.go`, `jsCheckRunning`: `running := []string{}` (не nil) → JSON `[]`.
- `ui/index.html`: в `doBackup` и `doRestore` добавлено `const running =
  check.running || [];` и проверка по `running`.

### Что проверено рабочим (на реальной машине, диагностической сборкой)

- Полный путь клика «Создать бэкап» (JS `doBackup` → Go `jsBackup` → копирование
  → проверка целостности) — работает, бэкап Chrome создан (15.1 МБ, 511 файлов).
- Новый `dialog.go`: создание, `setPickFolders`, `setDialogFolder`, `comRelease`
  — без падений (exit=0) для папок `D:\`, `D:\BrowserBackup`, `` (пусто).
- **ЕЩЁ НЕ ПРОВЕРЕНО В GUI:** реальный показ диалога (`showModal`) и клики
  «Открыть папку». См. раздел 4.

---

## 3. «Проверка целостности бесконечная» — РЕШЕНО

Корень: `doBackup`/`jsBackup` выполнялись **в потоке окна WebView2** (go-webview2
вызывает биндинг прямо из обработчика `WebMessageReceived` → `reflect.Call`),
а прогресс кладётся в очередь того же окна (`w.Dispatch`). Пока биндинг не
вернулся, цикл сообщений занят → окно «Не отвечает», прогресс не рисуется.
Плюс SHA-256 считался **дважды**: при копировании (`digestTree` по dst) и затем
`verifyManifest` перечитывал всё с диска.

Замерено: ~70 МБ/с. Для Zen 2.2 ГБ это ~40 с × 2 = ~1.5–2 мин.

**Что сделано (см. 2б):** хеш считается на лету при копировании (один проход),
backup/restore уведены в горутину, итог доставляется через `window.__opDone`.
Проверено на реальном Chrome-бэкапе (15.8 МБ, 511 файлов): окно живо, прогресс
идёт, сводки на лету точно совпали с независимым пересчётом.
**Ещё НЕ проверено:** бэкап БОЛЬШОГО профиля (Zen 2.2 ГБ) — сам эффект
ускорения на гигабайтах не замерялся, только корректность механизма.

---

## 4. Что делать дальше (план)

1. **Проверить в GUI руками** (главное):
   - «Изменить…» / «Добавить папку…» → должен открыться системный диалог
     (в прошлой сессии `dialog.go` переписан, но показ в GUI так и НЕ проверен);
   - «Открыть папку» → проводник;
   - «Создать бэкап» на большом профиле (Zen 2.2 ГБ) — окно не должно
     «зависать», прогресс должен идти, заодно замерить время;
   - бэкапы в списке теперь БЕЗ галочек по умолчанию (см. 2а) — отметить вручную;
   - восстановление (restore) — тоже ушло в горутину.
   Свежий exe: `/путь/к/BrowserBackup/` и
   `/путь/к/Downloads/#Test/`.
2. Проверить портативность в GUI: после запуска/закрытия в `%APPDATA%` не
   должно появляться папок `BrowserBackup*` (см. смоук в разделе 9).
3. Обновить `README.txt` при необходимости.
4. Пересобрать и разложить exe заново после любых правок.

**Уже сделано в конце сессии:** старые следы от прежних версий удалены —
`%APPDATA%\BrowserBackup`, `%APPDATA%\BrowserBackup.exe` (70 МБ),
`BrowserBackupDiag.exe`, `BrowserBackupDiagC.exe`. Проверено: ни в `%TEMP%`,
ни в `%LOCALAPPDATA%`, ни в реестре (`HKCU\Software\BrowserBackup`, `HKCU\...\Run`)
следов нет.

---

## 5. Как тестировать из WSL (приёмы, которые реально работали)

Запуск Windows-программы и захват вывода:

```sh
cd /mnt/c && powershell.exe -NoProfile -Command "Start-Process -FilePath 'C:\path\app.exe' -ArgumentList '-flag' -RedirectStandardError 'C:\путь\к\err.txt' -PassThru -Wait"
```

**ВАЖНО про этот окружение:**
- `cat` в zsh у пользователя — алиас на `bat`, которого нет. Не использовать
  `cat`/`sed`/`echo` в Bash-тулах; читать файлы инструментом Read, писать — Write.
- GUI-сборка с `-H windowsgui` глотает stdout/stderr → **панику не видно**.
  Для отладки собирать **без** `-H windowsgui` и с `-RedirectStandardError`.
- Скриншот окна: `PrintWindow` с флагом `2` (PW_RENDERFULLCONTENT) — см. приёмы
  в истории; окно должно быть на переднем плане.
- Журнал событий Windows у этого пользователя пуст на краши — не полагаться на него.
- Диагностическая копия проекта со всеми тестовыми флагами: `/tmp/bbdiag/`
  (флаги `-selftest`, `-auto`, `-autojs`, `-autodlg`, `-probe=`, `-nargs=`,
  `-slot=`). **ВНИМАНИЕ:** в `/tmp/bbdiag` СВОИ `gui.go` и `ui/index.html` с
  diag-хуками (`diagInit()` в начале `runApp`, таймеры `-auto/-autojs/-autodlg`,
  bind `diagLog`, JS-функция `diag()`). При синхронизации правок из проекта НЕ
  перезаписывать эти файлы целиком — иначе хуки теряются. `/tmp` может
  очиститься между сессиями — тогда пересоздать по этому описанию.

---

## 6. Memory-файлы

Путь: `<папка памяти Claude Code>`

- `browserbackup-project.md` — общий.
- `browserbackup-webview-bind-arity.md` — про точное число аргументов Bind.
- `browserbackup-dialog-sta.md` — про STA и поток окна (актуален: свой COM-диалог).
- `browserbackup-cfd-nargs-bug.md` — nargs-баг go-common-file-dialog.
- `browserbackup-json-null-traps.md` — JSON null из Go → падение JS.
- `browserbackup-ui-traps.md` — три ловушки GUI.
- `browserbackup-async-ops.md` — долгие операции в горутине + `__opDone`.
- `browserbackup-integrity-hash.md` — хеш на лету, один проход.
- `browserbackup-portable.md` — настройки рядом с exe, WebView2-данные во temp.
- `browserbackup-zen-profiles.md` — про два профиля Zen (Default Profile пустой).

**Что стоит сохранить в memory (по итогам всех сессий):**
- Причина падения диалога: `syscall.Syscall` с неверным `nargs` в
  `go-common-file-dialog` ломается на Go 1.17+ (amd64). Решение — свой COM-диалог
  через `syscall.SyscallN`. Это неочевидный, дорого найденный факт.
- `checkRunning` отдавал `null` вместо `[]` → JS падал на `.length`.
- **Биндинги go-webview2 выполняются В ПОТОКЕ ОКНА** (`WebMessageReceived` →
  `callbinding` → `reflect.Call`). Долгая работа в биндинге замораживает окно
  («Не отвечает») И блокирует доставку прогресса (он идёт через `w.Dispatch` в
  ту же очередь). Решение — горутина + доставка итога через отдельный
  JS-колбэк (`window.__opDone`), RPC возвращает `{}` сразу. Это ключ к
  «бесконечной проверке целостности».
- Проверка целостности: хеш можно считать на лету при копировании, тогда
  второго прохода по диску не нужно (~вдвое быстрее).
- Портативность: go-webview2 по умолчанию кладёт 70 МБ данных в
  `%APPDATA%\<имя exe>\EBWebView` — лечится `WebViewOptions.DataPath` во
  временную папку (см. [[browserbackup-portable]]).

**Все перечисленные факты УЖЕ внесены в memory-файлы** (включая
`browserbackup-async-ops`, `browserbackup-integrity-hash`,
`browserbackup-portable`). Отдельно обновлять не требуется.

---

## 7. Состояние файлов проекта (на момент передачи)

- `dialog.go` — **ПЕРЕПИСАН** (свой COM-диалог). Компилируется.
- `gui.go` — build-тег `//go:build windows`; `jsCheckRunning` (`[]string{}`);
  `jsBackup`/`jsRestore` асинхронные (горутина + `__opDone`); `progressFn`
  троттлится; `dispatchOpDone`; `errNoSelection`; WebView2 `DataPath` во
  временной папке + `removeWebViewData`/`cleanupStaleWebViewData`.
- `settings.go` — настройки РЯДОМ С .EXE (`BrowserBackup-settings.json`),
  `settingsDir()` удалена, `saveSettings` не падает на read-only.
- `util.go` — `exeDir()` (общий для настроек и browsers.json).
- `copy.go` — `copyFile` возвращает `(n, sha256, err)`; `copyTree` — сводку;
  хелпер `hashLines`.
- `integrity.go` — `digestTree` использует `hashLines` (для старых бэкапов).
- `backup.go` — сводки на лету, `verified:true` без второго прохода.
- `restore.go` — проверка только для непроверенных бэкапов.
- `ui/index.html` — `PENDING_OP`, `runOp`, `window.__opDone`; бэкапы БЕЗ `sel`
  по умолчанию (галочек при старте нет).
- Тесты: `copy_test.go`, `backup_test.go`, `settings_test.go` (под linux).
- `go.mod` — go-ole, go-webview2, golang.org/x/sys.
- Сборка: windows ок; `go test`/`go vet`/`gofmt` чисто (vet даёт только
  ожидаемый unsafe.Pointer-шум в `dialog.go`).
- Готовый билд: `/tmp/bb-check.exe` (может исчезнуть); разложен в
  `/путь/к/BrowserBackup/` и `/путь/к/Downloads/#Test/`.

---

## 8. Мини-промпт для нового чата

> Продолжаем работу над `/путь/к/browser-backup-tool` (см. `HANDOFF.md`
> в корне проекта — прочитай его целиком). Уже сделано: свой COM-диалог вместо
> сломанной `go-common-file-dialog` (nargs-баг), `checkRunning` null→[],
> backup/restore уведены в горутину (биндинги go-webview2 идут в потоке окна!),
> SHA-256 считается на лету при копировании, итог приходит в `window.__opDone`,
> программа полностью портативная (настройки рядом с exe, WebView2-данные во
> временной папке), галочки на бэкапах не проставляются сами. Нужно: проверить
> в GUI руками («Изменить…», «Добавить папку…», «Открыть папку», «Создать
> бэкап» на большом профиле — окно не должно зависать) и проверить restore.
> Говори со мной по-русски. Учти ловушки окружения из раздела 5 HANDOFF.md
> (cat=bat, паника не видна в windowsgui-сборке и т.п.).

---

## 9. Проверка «на пальцах» после перезапуска

Быстрый смоук-тест, что ничего не сломалось:

```sh
export PATH=$PATH:/путь/к/go-toolchain/go/bin GOPATH=/путь/к/go-toolchain/gopath
cd /путь/к/browser-backup-tool
GOOS=linux GOFLAGS=-mod=mod go test . && echo TESTS_OK          # логика (хеши/манифест/настройки)
GOOS=windows GOARCH=amd64 GOFLAGS=-mod=mod go build -ldflags "-s -w -H windowsgui" -o /tmp/bb-check.exe . && echo BUILD_OK
```

Проверка портативности (что следов нет):

```sh
# запустить exe из отдельной папки, затем:
cd /mnt/c && powershell.exe -NoProfile -Command "
Get-ChildItem \$env:APPDATA -Directory | Where-Object { \$_.Name -like '*BrowserBackup*' }   # НЕ должно быть новых
Get-ChildItem \$env:TEMP -Directory | Where-Object { \$_.Name -like 'BrowserBackup-webview-*' }  # пусто после закрытия
"
```

Затем запустить на Windows и вручную проверить кнопки. Для автопрогона есть
диагностический стенд `/tmp/bbdiag` (флаг `-autojs` прогоняет doBackup в реальном
UI и пишет `%USERPROFILE%\bb-diag.log`). **ВАЖНО:** в `/tmp/bbdiag` свой `gui.go`
и `ui/index.html` (с diag-мостом `diagLog`/`diag()`); НЕ перезаписывать их
файлами из проекта целиком — при синхронизации правок надо сохранять diag-хуки
(`diagInit()` в начале `runApp`, таймеры `-auto/-autojs/-autodlg`, bind `diagLog`,
JS-функцию `diag()`).

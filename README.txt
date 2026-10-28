========================================================================
 BrowserBackup.exe — бэкап профилей браузеров
 Browser Profile Backup — portable, без установки
========================================================================

Что это
-------
Одна portable-программа (один .exe), которая сохраняет профили браузеров
и восстанавливает их после переустановки Windows. Ничего не устанавливает
в систему и не оставляет следов, кроме самих бэкапов.

Поддерживаются (определяются автоматически):
  • Firefox-семейство: Zen, Firefox, LibreWolf, Waterfox, Floorp
  • Chromium-семейство: Chrome, Edge, Brave, Vivaldi, Chromium, Thorium,
    Ungoogled Chromium, SRWare Iron, Helium, Arc
  • Любой другой браузер — режим «Другой браузер» (указать папку профиля вручную).

Интерфейс современный (на движке WebView2, встроен в Windows 10/11),
тёмно-синяя тема, английский и русский язык (английский включён по умолчанию).
Каждый браузер — отдельным блоком; нажмите на него, чтобы раскрыть список
его профилей и отметить нужные галочками.

ВАЖНО: WebView2 Runtime нужен для окна. В Windows 11 и большинстве сборок
Windows 10 он уже установлен. Если окно не открывается — установите
«Microsoft Edge WebView2 Runtime» с сайта Microsoft.


ПРИ ПЕРВОМ ЗАПУСКЕ: SmartScreen и антивирус
--------------------------------------------
Файл не имеет платной цифровой подписи, поэтому при первом запуске Windows
может показать синее окно «Система Windows защитила ваш компьютер»
(SmartScreen), а антивирус — предупреждение о «неизвестном издателе».

Это нормально для бесплатной portable-программы. Что делать:
  • SmartScreen: «Подробнее» → «Выполнить в любом случае».
  • Антивирус: добавить файл в исключения.
Программа ничего не скачивает из интернета и не меняет систему — только
копирует файлы профилей туда, куда вы укажете.


ЧТО НЕ КОПИРУЕТСЯ (И ПОЧЕМУ)
----------------------------------
Кеш браузера не копируется: для переноса данных он не нужен и занимает
много места.

Отдельно пропускается файловое хранилище сайтов — папки вида
storage/<сайт>/fs. Сайты складывают туда произвольные файлы: офлайн-копии,
ресурсы игр, скачанные данные. На реальном профиле Zen это 695 МБ из
1043 МБ, то есть две трети бэкапа. К настройкам, паролям и кукам эти
файлы отношения не имеют.

IndexedDB (idb) и localStorage (ls) при этом КОПИРУЮТСЯ — именно там
лежит состояние веб-приложений и входы на сайты.


ВАЖНО ПРО ПАРОЛИ И ВХОДЫ НА САЙТЫ
----------------------------------
Это ключевое различие между семействами браузеров:

* Firefox-семейство (Zen, Firefox, LibreWolf, Waterfox, Floorp) — пароли и
  входы ХРАНЯТСЯ В ПРОФИЛЕ и переносятся файлами. После восстановления вы
  будете залогинены везде, как раньше.

* Chromium-семейство (Chrome, Edge, Brave, Vivaldi, Chromium, Thorium,
  Ungoogled Chromium, SRWare Iron, Helium, Arc) — пароли и куки зашифрованы ключом
  вашей учётной записи Windows. После переустановки Windows этот ключ
  меняется, поэтому старые пароли и куки НЕ РАСШИФРУЮТСЯ, даже если файлы
  скопированы идеально.
  Файлами переносятся: закладки, история, расширения, настройки.
  Пароли и входы — ТОЛЬКО через вход в аккаунт (Google/Microsoft/Brave/…)
  и включённую синхронизацию; тогда они подтянутся из аккаунта.

  То есть для Chromium-браузеров «везде залогинен» получится через
  синхронизацию аккаунта, а не через файлы. Программа предупреждает
  об этом прямо в окне, рядом с каждым таким браузером.


Как пользоваться
----------------
1. Запустите BrowserBackup.exe (двойной клик).
2. В разделе «Браузеры и профили» нажмите на нужный браузер — раскроется
   список его профилей. Отметьте галочками нужные.
   • Профиль с пометкой «активный» — тот, которым вы пользуетесь сейчас.
   • Пометка «пустой» — служебный профиль без данных (его бэкапить не нужно).
     Например, у Zen рядом с рабочим профилем может лежать пустая заготовка
     «Default Profile» — это НЕ дубликат, а служебная папка без данных;
     бэкапить нужно профиль с данными (у Zen это обычно «Default (release)»).
3. Укажите «Папку для бэкапа» кнопкой «Изменить…».
   ВАЖНО: не храните бэкап на диске C: — при переустановке Windows
   он сотрётся. Программа об этом предупредит.
4. Нажмите «Создать бэкап».
   Браузер должен быть ЗАКРЫТ — иначе часть файлов не скопируется,
   и программа не даст продолжить.

После создания бэкапа программа САМА проверяет целостность: контрольные
суммы (SHA-256) каждого файла считаются прямо при копировании, поэтому
проверка не требует отдельного прохода и не замедляет работу. Если копия
получилась битой, вы увидите предупреждение, а не «тихо сломанный» бэкап.

Если нужного браузера нет в списке — нажмите «Добавить папку…» и укажите
папку его профиля вручную (режим «Другой браузер»). Папка переносится
целиком, как есть.


Восстановление (после переустановки Windows)
--------------------------------------------
1. Установите браузер(ы) и запустите каждый ОДИН РАЗ, затем закройте.
   Это нужно, чтобы браузер сам создал папку профиля с новым id.
2. Запустите BrowserBackup.exe, укажите ту же папку бэкапа.
3. В разделе «Восстановить из бэкапа» отметьте нужные бэкапы.
4. Нажмите «Восстановить» и подтвердите.

Программа НЕ удаляет текущий профиль: сначала делает страховочную копию
(в подпапку _safety), потом заменяет содержимое. Откатиться можно в любой
момент — просто скопируйте данные из _safety обратно.

Перед восстановлением каждый бэкап тоже проверяется на целостность
(для бэкапов, проверенных при создании, повторная проверка не нужна).
Битые бэкапы пропускаются, чтобы не «окирпичить» браузер.


Особенности
-----------
* Профиль переносится в папку, которую создал НОВЫЙ браузер, а не в папку
  со старым случайным id. Так работает корректно.
* Кэш не копируется (он не нужен для переноса закладок, паролей, входов
  и настроек, а места занимает много).
* Каждый бэкап — отдельная папка с датой и временем в имени
  (например zen_Default (release)_2026-10-09_18-41-12). Новый бэкап
  НИКОГДА не перезаписывает старый.
* Внутри каждого бэкапа — файл manifest.json и README.txt (на русском — ПРОЧТИ_МЕНЯ.txt) с описанием:
  браузер и его версия, профиль, дата, версия Windows, компьютер,
  пользователь, число файлов, размер, контрольные суммы.
* Все действия видны в журнале в самом окне программы. «Открыть папку» —
  открывает папку бэкапа в проводнике. Файл журнала в папке бэкапа не
  создаётся: программа не оставляет рядом с бэкапами ничего лишнего.
* Понятные сообщения об ошибках вместо кодов.


Настройки и список браузеров
----------------------------
Программа полностью портативная и НЕ оставляет следов в системе: ни в
реестре, ни в профиле пользователя (%APPDATA%). Настройки хранятся рядом
с программой, в файле:

    BrowserBackup-settings.json   (в папке, где лежит .exe)

Там сохраняются выбранная папка бэкапа, язык и папки «Другого браузера».
Папку с .exe можно целиком перенести на флешку — настройки переедут с ней.
Удалите этот файл — настройки сбросятся к значениям по умолчанию.

Временные данные WebView2 (кэш движка, ~десятки МБ) программа держит во
временной папке Windows и удаляет при выходе — в профиле пользователя
после неё ничего не остаётся.

СПИСОК БРАУЗЕРОВ тоже хранится в данных, а не в коде. Программа НЕ создаёт
этот файл сама; если вы хотите дополнить список — положите рядом с .exe файл
browsers.json, и он будет использован вместо встроенного списка. Формат:

    {
      "browsers": [
        { "key": "mybrowser", "name": "My Browser", "family": "firefox",
          "proc": ["mybrowser"], "roaming": "MyBrowser", "local": "MyBrowser" },
        { "key": "other", "name": "Other", "family": "chromium",
          "proc": ["other"], "local": "Other/User Data" }
      ]
    }

  family: "firefox" (профили в %APPDATA%, пароли файлами) или
          "chromium" (профили в %LOCALAPPDATA%, пароли только аккаунтом).
  roaming / local / packages — пути-кандидаты (можно несколько через "|").
  Если файла нет или он повреждён, используется встроенный список.


Совет по безопасности
---------------------
Для Firefox-семейства бэкап содержит пароли и активные сессии — это фактически
ключи от ваших аккаунтов. Не выкладывайте папку бэкапа в облако и
мессенджеры. Для надёжности держите её на зашифрованном диске
(BitLocker/VeraCrypt).

------------------------------------------------------------------------
ENGLISH
------------------------------------------------------------------------
A single portable .exe that backs up browser profiles and restores them
after reinstalling Windows. No installation, no traces left in the system
except the backups.

Detected automatically:
  • Firefox family: Zen, Firefox, LibreWolf, Waterfox, Floorp
  • Chromium family: Chrome, Edge, Brave, Vivaldi, Chromium, Thorium,
    Ungoogled Chromium, SRWare Iron, Helium, Arc
  • Any other browser — via "Other browser" (pick a profile folder manually).

The window uses WebView2 (built into Windows 10/11). On first launch
SmartScreen or your antivirus may warn about an unknown publisher — this
is normal for a free unsigned portable app. Choose "More info" →
"Run anyway", or add it to your antivirus exclusions.

PASSWORDS — IMPORTANT:
* Firefox family stores passwords IN the profile — they transfer with files.
* Chromium family (Chrome, Edge, Brave, Vivaldi, …) encrypts passwords and
  cookies with your Windows account key. After reinstalling Windows the key
  changes, so old passwords and cookies will NOT decrypt even if files are
  copied perfectly. Files carry bookmarks, history, extensions and settings;
  sign-ins and passwords come back only via account sign-in (sync).
  The app warns about this in the window, next to each such browser.

Backup:  expand a browser -> pick profiles -> choose a folder (NOT on C:)
         -> Create backup. The browser must be closed. Integrity (SHA-256)
         is verified automatically.
Restore: install the browser, run it once and close it (so it creates a
         fresh profile folder), then pick the backup -> Restore. A safety
         copy of the current profile is made first; nothing is deleted.
         Damaged backups are skipped.
Log:     in the window only; the temporary log file lives in the OS temp
         folder and is deleted on exit (nothing is left in the backup folder).
Config:  fully portable — settings live in BrowserBackup-settings.json next
         to the .exe, nothing is written to %APPDATA% or the registry. Carry
         the folder with the .exe anywhere and the settings travel with it.
         To extend the browser list, put an optional browsers.json next to
         the .exe. WebView2 temp data goes to the OS temp folder and is
         removed on exit.
Language: English/Russian — English by default, switch in the window.

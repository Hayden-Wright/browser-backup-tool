//go:build windows

package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jchv/go-webview2"
)

//go:embed ui/index.html
var uiHTML string

//go:embed build/icon.png
var appIconPNG []byte

// App — состояние приложения и мост в JavaScript.
type App struct {
	w        webview2.WebView
	settings Settings
	cancel   *atomic.Bool
	busy     bool
	mu       sync.Mutex
	lastProg atomic.Int64 // время последнего обновления прогресса (мс)
}

// runApp запускает окно.
func runApp() {
	app := &App{settings: loadSettings(), cancel: &atomic.Bool{}}
	currentLang = parseLang(app.settings.Language)

	// Portable: данные WebView2 (кэш, профиль движка) кладём во временную
	// папку, а не в %APPDATA%. По умолчанию go-webview2 создаёт
	// %APPDATA%\<имя exe>\EBWebView — десятки МБ мусора в профиле
	// пользователя. Временную папку удаляем при выходе.
	cleanupStaleWebViewData()
	dataPath, err := os.MkdirTemp("", "BrowserBackup-webview-")
	if err != nil {
		dataPath = "" // крайний случай — пусть WebView2 выберет сам
	}
	defer removeWebViewData(dataPath)

	// Окно должно появиться уже без системной шапки и с готовым содержимым:
	// хук правит стиль в момент создания, а показываем окно мы сами (см. uiReady).
	unhook := installCreateHook()
	defer unhook()

	app.w = webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  dataPath,
		WindowOptions: webview2.WindowOptions{
			Title:  T("app_title"),
			Width:  900,
			Height: 780,
		},
	})
	if app.w == nil {
		return // нет WebView2 Runtime
	}
	defer app.w.Destroy()

	// Привязка функций, доступных из JS как window.api.*
	app.w.Bind("state", app.jsState)
	app.w.Bind("pickFolder", app.jsPickFolder)
	app.w.Bind("pollFolder", app.jsPollFolder)
	app.w.Bind("setDir", app.jsSetDir)
	app.w.Bind("openFolder", app.jsOpenFolder)
	app.w.Bind("openLog", app.jsOpenLog)
	app.w.Bind("checkRunning", app.jsCheckRunning)
	app.w.Bind("backup", app.jsBackup)
	app.w.Bind("restore", app.jsRestore)
	app.w.Bind("setLanguage", app.jsSetLanguage)
	app.w.Bind("addCustomDir", app.jsAddCustomDir)
	app.w.Bind("removeCustomDir", app.jsRemoveCustomDir)
	app.w.Bind("windowAction", app.jsWindowAction)
	app.w.Bind("setDragArea", app.jsSetDragArea)
	app.w.Bind("startDrag", app.jsStartDrag)
	app.w.Bind("ready", app.jsReady)

	// Журнал идёт и в файл, и в окно.
	log.SetSink(app.logSink)
	log.SetFile(app.settings.BackupDir)
	log.Log("%s", Tf("log_ready", app.settings.BackupDir))

	// Стиль уже без заголовка (хук), но подкласс для перетаскивания ставим
	// здесь: он нужен на всё время жизни окна.
	makeFrameless(app.hwnd())

	app.w.SetHtml(uiHTML)
	app.w.Run()
}

// removeWebViewData удаляет временную папку данных WebView2 при выходе.
// WebView2 может держать файлы ещё пару секунд после Destroy, поэтому
// пробуем несколько раз.
func removeWebViewData(dir string) {
	if dir == "" {
		return
	}
	for i := 0; i < 20; i++ {
		if err := os.RemoveAll(dir); err == nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// cleanupStaleWebViewData убирает временные папки WebView2, оставшиеся от
// предыдущих запусков (например, если программа была завершена аварийно и
// defer не сработал). Свежие папки (моложе минуты) не трогаем — их может
// держать другой запущенный экземпляр.
func cleanupStaleWebViewData() {
	base := os.TempDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Minute)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "BrowserBackup-webview-") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		os.RemoveAll(filepath.Join(base, e.Name()))
	}
}

// ---------- состояние ----------

type jsBrowser struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Family   string   `json:"family"`
	Version  string   `json:"version"`
	Password string   `json:"password"`
	Profiles []jsProf `json:"profiles"`
}

type jsProf struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	IsActive  bool   `json:"isActive"`
	IsDefault bool   `json:"isDefault"`
	IsEmpty   bool   `json:"isEmpty"`
	Path      string `json:"path,omitempty"` // для режима «Другой браузер»
}

type jsBackup struct {
	Browser    string `json:"browser"`
	Profile    string `json:"profile"`
	Created    string `json:"created"`
	Folder     string `json:"folder"`
	BrowserKey string `json:"browser_key"`
	Verified   *bool  `json:"verified"` // nil — неизвестно
}

func (a *App) jsState() (string, error) {
	// Сбор состояния (обход профилей, подсчёт размеров) может быть долгим.
	// Выполняем его ВНЕ потока окна: иначе цикл сообщений окна блокируется
	// и окно на несколько секунд зависает в состоянии «Не отвечает».
	done := make(chan string, 1)
	go func() { done <- a.buildState() }()
	select {
	case s := <-done:
		return s, nil
	case <-time.After(120 * time.Second):
		return "{}", nil
	}
}

func (a *App) buildState() string {
	browsers := []jsBrowser{}
	for _, g := range browserGroups() {
		jb := jsBrowser{Key: g.Key, Name: g.Name, Family: g.Family, Version: g.Version, Password: g.Password, Profiles: []jsProf{}}
		for _, p := range g.Profiles {
			jp := jsProf{
				Name: p.Name, Size: p.SizeBytes, IsActive: p.IsActive,
				IsDefault: p.IsDefault, IsEmpty: p.IsEmpty,
			}
			if g.Family == "custom" {
				jp.Path = p.RelPath
			}
			jb.Profiles = append(jb.Profiles, jp)
		}
		browsers = append(browsers, jb)
	}

	backups := []jsBackup{}
	for _, m := range listBackups(a.settings.BackupDir) {
		// НЕ пересчитываем хеши при каждом открытии: берём сохранённый
		// результат проверки. Если его нет (старый бэкап) — «неизвестно».
		backups = append(backups, jsBackup{
			Browser: m.BrowserName, Profile: m.ProfileName,
			Created: humanCreated(m.Created), Folder: m.Folder,
			BrowserKey: m.BrowserKey, Verified: m.Verified,
		})
	}

	out := map[string]any{
		"browsers": browsers,
		"backups":  backups,
		"dir":      a.settings.BackupDir,
		"lang":     langCode(currentLang),
	}
	data, _ := json.Marshal(out)
	return string(data)
}

// humanCreated превращает "2026-10-09_17-22-26" в "2026-10-09 17:22".
func humanCreated(s string) string {
	s = strings.Replace(s, "_", " ", 1)
	if len(s) >= 16 {
		return s[:16]
	}
	return s
}

// ---------- папка ----------

// hwnd — дескриптор окна (для родителя системных диалогов).
func (a *App) hwnd() uintptr {
	if a.w == nil {
		return 0
	}
	return uintptr(a.w.Window())
}

// jsSetDir меняет папку бэкапа. Сигнатура ровно с одним аргументом:
// webview2 требует точного совпадения числа аргументов с JS-вызовом.
func (a *App) jsSetDir(dir string) (string, error) {
	if dir == "" {
		return "{}", nil
	}
	a.settings.BackupDir = dir
	saveSettings(a.settings)
	log.SetFile(dir)
	log.Log("Backup folder: %s", dir)
	return "{}", nil
}

func (a *App) jsOpenFolder() (string, error) {
	dir := a.settings.BackupDir
	if dir == "" {
		return "{}", nil
	}
	go openPath(dir) // запуск проводника вне потока окна
	return "{}", nil
}

func (a *App) jsOpenLog() (string, error) {
	p := log.Path()
	if p != "" && pathExists(p) {
		go openPath(p)
	}
	return "{}", nil
}

func (a *App) jsSetLanguage(lang string) (string, error) {
	a.settings.Language = lang
	saveSettings(a.settings)
	currentLang = parseLang(lang)
	return "{}", nil
}

// ---------- своя шапка окна ----------

// jsWindowAction — кнопки своей шапки: свернуть, развернуть, закрыть.
// Одна функция с аргументом, а не три отдельные: webview2 требует точного
// совпадения числа аргументов, и лишние привязки ничего не дают.
func (a *App) jsWindowAction(action string) (string, error) {
	windowAction(a.hwnd(), action)
	return "{}", nil
}

// jsReady вызывается из JS, когда интерфейс отрисован. Окно к этому моменту
// уже видно (скрывать его нельзя — WebView2 не рендерит скрытое окно), но
// это полезная точка: сюда можно повесить отложенные действия, которым нужен
// готовый DOM.
func (a *App) jsReady() (string, error) {
	return "{}", nil
}

// jsStartDrag начинает перетаскивание окна — вызывается из JS по нажатию
// на шапку. Почему из JS, а не через WM_NCHITTEST, см. startDrag.
func (a *App) jsStartDrag() (string, error) {
	startDrag()
	return "{}", nil
}

// jsSetDragArea сообщает Go, где находится шапка, чтобы окно таскалось
// за неё. Координаты в CSS-пикселях; при другом масштабе экрана WebView2
// пересчитывает их сам, поэтому передаём как есть.
func (a *App) jsSetDragArea(height, excludeRight int) (string, error) {
	setDragArea(height, excludeRight)
	return "{}", nil
}

// jsAddCustomDir — режим «Другой браузер»: добавить папку профиля по пути
// (путь получен диалогом через pickFolder/pollFolder и передан из JS).
func (a *App) jsAddCustomDir(dir string) (string, error) {
	if dir == "" {
		return "{}", nil
	}
	// не добавляем повторно
	for _, d := range a.settings.CustomProfileDirs {
		if strings.EqualFold(d, dir) {
			return "{}", nil
		}
	}
	a.settings.CustomProfileDirs = append(a.settings.CustomProfileDirs, dir)
	saveSettings(a.settings)
	return "{}", nil
}

// jsRemoveCustomDir — убрать папку из режима «Другой браузер».
func (a *App) jsRemoveCustomDir(dir string) (string, error) {
	var kept []string
	for _, d := range a.settings.CustomProfileDirs {
		if !strings.EqualFold(d, dir) {
			kept = append(kept, d)
		}
	}
	a.settings.CustomProfileDirs = kept
	saveSettings(a.settings)
	return "{}", nil
}

// ---------- проверка запущенных ----------

func (a *App) jsCheckRunning(keys []string) (string, error) {
	seen := map[string]bool{}
	running := []string{} // не nil: в JSON это [] (не null), JS читает .length
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		if def := browserByKey(k); def != nil && def.isRunning() {
			running = append(running, def.spec.Name)
		}
	}
	res := map[string]any{"running": running}
	data, _ := json.Marshal(res)
	return string(data), nil
}

// ---------- операции ----------

type selProf struct {
	B int `json:"b"`
	P int `json:"p"`
}

func (a *App) jsBackup(sel []selProf) (string, error) {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return errJSON("error", "busy"), nil
	}
	a.busy = true
	cancel := &atomic.Bool{}
	a.cancel = cancel
	a.mu.Unlock()

	// Копирование и хеширование больших профилей — долгая операция. Биндинг
	// выполняется В ПОТОКЕ ОКНА (go-webview2 вызывает его прямо из обработчика
	// сообщений), поэтому запускаем работу в отдельной горутине: иначе цикл
	// сообщений окна блокируется, окно уходит в «Не отвечает», а прогресс,
	// который кладётся в очередь того же окна, не отрисовывается. Результат
	// доставляем в JS отдельным вызовом window.__opDone.
	go func() {
		defer func() {
			a.mu.Lock()
			a.busy = false
			a.mu.Unlock()
		}()

		res, err := func() (*BackupResult, error) {
			groups := browserGroups()
			var profs []Profile
			for _, s := range sel {
				if s.B < len(groups) && s.P < len(groups[s.B].Profiles) {
					profs = append(profs, groups[s.B].Profiles[s.P])
				}
			}
			if len(profs) == 0 {
				return nil, errNoSelection
			}
			log.Log(T("log_started"), T("create_backup"))
			return doBackup(profs, a.settings.BackupDir, cancel, a.progressFn)
		}()

		out := map[string]any{"kind": "backup"}
		switch {
		case errors.Is(err, errNoSelection):
			out["error"] = T("no_selection")
		case errors.Is(err, errCancelled):
			out["cancelled"] = true
		case err != nil:
			log.Log(T("log_error"), err.Error())
			out["error"] = friendlyError(err)
		default:
			out["count"] = res.Count
			out["bytes"] = res.Bytes
			out["broken"] = res.Broken
			out["skipped"] = res.Skipped
		}
		a.dispatchOpDone(out)
	}()
	return "{}", nil
}

func (a *App) jsRestore(folders []string) (string, error) {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return errJSON("error", "busy"), nil
	}
	a.busy = true
	cancel := &atomic.Bool{}
	a.cancel = cancel
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.busy = false
			a.mu.Unlock()
		}()

		res, err := func() (*RestoreResult, error) {
			all := listBackups(a.settings.BackupDir)
			want := map[string]bool{}
			for _, f := range folders {
				want[f] = true
			}
			var sel []manifest
			for _, m := range all {
				if want[m.Folder] {
					sel = append(sel, m)
				}
			}
			if len(sel) == 0 {
				return nil, errNoSelection
			}
			log.Log(T("log_started"), T("restore"))
			return doRestore(sel, a.settings.BackupDir, cancel, a.progressFn)
		}()

		out := map[string]any{"kind": "restore"}
		switch {
		case errors.Is(err, errNoSelection):
			out["error"] = T("nothing_backup")
		case errors.Is(err, errCancelled):
			out["cancelled"] = true
		case err != nil:
			log.Log(T("log_error"), err.Error())
			out["error"] = friendlyError(err)
		default:
			out["count"] = res.Count
			out["bytes"] = res.Bytes
			out["safety"] = res.SafetyDir
			out["notRun"] = res.NotRun
			out["broken"] = res.Broken
		}
		a.dispatchOpDone(out)
	}()
	return "{}", nil
}

// dispatchOpDone доставляет итог долгой операции в JS (window.__opDone).
func (a *App) dispatchOpDone(out map[string]any) {
	if a.w == nil {
		return
	}
	payload, _ := json.Marshal(out)
	js := "window.__opDone(" + string(payload) + ")"
	a.w.Dispatch(func() { a.w.Eval(js) })
}

// progressFn шлёт прогресс в интерфейс (не чаще ~10 раз в секунду).
func (a *App) progressFn(done, total int64, name string) {
	if a.w == nil {
		return
	}
	now := time.Now().UnixMilli()
	if last := a.lastProg.Load(); last != 0 && now-last < 100 {
		return
	}
	a.lastProg.Store(now)
	payload, _ := json.Marshal(map[string]any{"done": done, "total": total, "name": name})
	js := "window.__progress(" + string(payload) + ")"
	a.w.Dispatch(func() { a.w.Eval(js) })
}

// logSink шлёт строки журнала в интерфейс.
func (a *App) logSink(line string) {
	if a.w == nil {
		return
	}
	payload, _ := json.Marshal(line)
	js := "window.__log(" + string(payload) + ")"
	a.w.Dispatch(func() { a.w.Eval(js) })
}

func errJSON(k, v string) string {
	data, _ := json.Marshal(map[string]any{k: v})
	return string(data)
}

// errNoSelection — во внутренних горутинах jsBackup/jsRestore: выбранных
// элементов нет (в JS это разные тексты, поэтому разбирается по месту).
var errNoSelection = errors.New("no selection")

// isDriveC сообщает, что путь находится на диске C:.
func isDriveC(path string) bool {
	vol := strings.ToUpper(filepathVolume(path))
	return vol == "C:"
}

// filepathVolume извлекает букву диска ("C:") из пути.
func filepathVolume(path string) string {
	if len(path) >= 2 && path[1] == ':' {
		return path[:2]
	}
	return ""
}

// openPath открывает папку или файл в проводнике.
func openPath(p string) {
	openInExplorer(p)
}

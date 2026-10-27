package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Kind — тип хранилища профилей (совместимость с прежним кодом).
type Kind = Family

const (
	FirefoxLike  = FamilyFirefox
	ChromiumLike = FamilyChromium
)

// SourceItem — один источник данных профиля.
type SourceItem struct {
	Label  string // папка внутри бэкапа: Roaming / Local / Profile / Root
	Path   string // абсолютный путь-источник
	IsFile bool
}

// Profile — конкретный профиль браузера.
type Profile struct {
	ID          string
	Name        string
	BrowserKey  string
	BrowserName string
	Kind        Family
	RelPath     string
	IsDefault   bool
	IsActive    bool
	IsEmpty     bool
	SizeBytes   int64
	Sources     []SourceItem
}

func (p *Profile) DisplayName() string { return p.BrowserName + " — " + p.Name }

// BrowserGroup — браузер со своими профилями (для интерфейса).
type BrowserGroup struct {
	Key      string    `json:"key"`
	Name     string    `json:"name"`
	Family   string    `json:"family"`
	Version  string    `json:"version"`
	Password string    `json:"password"` // "account" | "files"
	Profiles []Profile `json:"profiles"`
}

// resolved — конкретные пути установленного браузера.
type resolved struct {
	spec        BrowserSpec
	roamingRoot string
	localRoot   string
	userData    string // для Chromium: папка User Data
	packaged    bool   // упакованное Store-приложение (Arc)
}

// resolveBrowser находит пути установленного браузера (или nil, если не установлен).
func resolveBrowser(spec BrowserSpec) *resolved {
	r := &resolved{spec: spec}
	appdata := os.Getenv("APPDATA")
	local := os.Getenv("LOCALAPPDATA")

	if spec.Family == FamilyFirefox {
		r.roamingRoot = firstExisting(appdata, spec.Roaming)
		r.localRoot = firstExisting(local, spec.Local)
		if r.roamingRoot == "" {
			return nil
		}
		return r
	}

	// Chromium-семейство
	if p := resolvePackages(local, spec.Packages); p != "" {
		r.userData = p
		r.packaged = true
		return r
	}
	if u := firstExisting(local, spec.Local); u != "" {
		r.userData = u
		return r
	}
	return nil
}

// firstExisting возвращает первый существующий путь из списка, разделённого "|".
func firstExisting(base, candidates string) string {
	if candidates == "" || base == "" {
		return ""
	}
	for _, c := range strings.Split(candidates, "|") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		p := filepath.Join(base, filepath.FromSlash(c))
		if pathExists(p) {
			return p
		}
	}
	return ""
}

// resolvePackages ищет упакованное приложение по glob под Packages.
func resolvePackages(local, pattern string) string {
	if pattern == "" || local == "" {
		return ""
	}
	full := filepath.Join(local, "Packages", filepath.FromSlash(pattern))
	matches, _ := filepath.Glob(full)
	for _, m := range matches {
		if pathExists(m) {
			return m
		}
	}
	return ""
}

// installedBrowsers возвращает список установленных браузеров.
func installedBrowsers() []*resolved {
	var out []*resolved
	for _, spec := range browserTable {
		if r := resolveBrowser(spec); r != nil {
			out = append(out, r)
		}
	}
	return out
}

// browserByKey находит установленный браузер по ключу.
func browserByKey(key string) *resolved {
	for _, r := range installedBrowsers() {
		if r.spec.Key == key {
			return r
		}
	}
	// даже если браузер не установлен, вернём спецификацию (для восстановления).
	for _, spec := range browserTable {
		if spec.Key == key {
			return &resolved{spec: spec}
		}
	}
	return nil
}

// procNames — имена процессов браузера.
func (r *resolved) procNames() []string { return r.spec.Proc }

// isRunning сообщает, запущен ли этот браузер.
//
// Если у браузера задан ProcPath, решает полный путь к .exe: Helium и Chrome
// называются одинаково (chrome.exe), и проверка только по имени давала бы
// ложное срабатывание в обе стороны. Если пути процессов получить не удалось,
// остаётся проверка по имени — см. runningByProcPath.
func (r *resolved) isRunning() bool {
	if r.spec.ProcPath == "" {
		return isRunning(r.spec.Proc...)
	}
	if running, decided := runningByProcPath(r.spec.ProcPath, runningExePaths()); decided {
		return running
	}
	// Пути процессов недоступны — откатываемся на имя. Для Helium это даёт
	// ложное «запущен» при открытом Chrome, но это безопасная сторона ошибки:
	// пользователь закроет браузер и повторит. Обратная ошибка (пропустить
	// открытый браузер) даёт битый бэкап, который обнаружится только при
	// восстановлении, — то есть когда данные уже потеряны.
	return isRunning(r.spec.Proc...)
}

// runningByProcPath решает, запущен ли браузер с указанным ProcPath, по списку
// путей запущенных процессов.
//
// decided == false означает, что список пуст и решение принять нельзя —
// вызывающий код должен откатиться на проверку по имени. Пустой список — это
// не «ничего не запущено», а «пути получить не удалось» (снимок процессов не
// создался, нет прав).
//
// Вынесено отдельной функцией, чтобы проверялось тестами на любой ОС:
// runningExePaths на Linux — заглушка, возвращающая nil.
func runningByProcPath(procPath string, paths []string) (running, decided bool) {
	if len(paths) == 0 {
		return false, false
	}
	want := normalizePath(procPath)
	for _, p := range paths {
		if strings.Contains(p, want) {
			return true, true
		}
	}
	return false, true
}

// isInstalled — найден ли браузер на диске.
func (r *resolved) isInstalled() bool {
	if r.spec.Family == FamilyFirefox {
		return r.roamingRoot != ""
	}
	return r.userData != ""
}

// passwordMode — способ переноса паролей: "files" или "account".
func (r *resolved) passwordMode() string {
	if r.spec.Family == FamilyFirefox {
		return "files"
	}
	return "account"
}

// discoverProfiles находит профили браузера.
func (r *resolved) discoverProfiles() []Profile {
	if r.spec.Family == FamilyFirefox {
		return r.discoverFirefoxLike()
	}
	return r.discoverChromiumLike()
}

// browserVersion пытается определить версию установленного браузера.
func (r *resolved) browserVersion() string {
	if r.spec.Family == FamilyChromium {
		if data, err := os.ReadFile(filepath.Join(r.userData, "Last Version")); err == nil {
			return strings.TrimSpace(string(data))
		}
		return ""
	}
	for _, p := range r.discoverProfiles() {
		for _, src := range p.Sources {
			if data, err := os.ReadFile(filepath.Join(src.Path, "compatibility.ini")); err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					if strings.HasPrefix(line, "LastVersion=") {
						return strings.TrimSpace(strings.TrimPrefix(line, "LastVersion="))
					}
				}
			}
		}
		break
	}
	return ""
}

// ---------- Firefox-like ----------

type iniProfile struct {
	Name    string
	Path    string
	Default bool
}

func (r *resolved) discoverFirefoxLike() []Profile {
	entries := parseProfilesIni(filepath.Join(r.roamingRoot, "profiles.ini"))
	active := r.activeProfilePath()

	var out []Profile
	for _, e := range entries {
		if e.Path == "" {
			continue
		}
		roaming := filepath.Join(r.roamingRoot, filepath.FromSlash(e.Path))
		local := filepath.Join(r.localRoot, filepath.FromSlash(e.Path))

		var sources []SourceItem
		if pathExists(roaming) {
			sources = append(sources, SourceItem{Label: "Roaming", Path: roaming})
		}
		if pathExists(local) {
			sources = append(sources, SourceItem{Label: "Local", Path: local})
		}
		if len(sources) == 0 {
			continue
		}

		empty := !pathExists(filepath.Join(roaming, "prefs.js")) &&
			!pathExists(filepath.Join(roaming, "logins.json"))

		name := e.Name
		if name == "" {
			name = friendlyProfileName(e.Path)
		}
		out = append(out, Profile{
			ID:          r.spec.Key + "|" + e.Path,
			Name:        name,
			BrowserKey:  r.spec.Key,
			BrowserName: r.spec.Name,
			Kind:        FamilyFirefox,
			RelPath:     e.Path,
			IsDefault:   e.Default,
			IsActive:    e.Path == active,
			IsEmpty:     empty,
			Sources:     sources,
		})
	}
	return out
}

func (r *resolved) activeProfilePath() string {
	if data, err := os.ReadFile(filepath.Join(r.roamingRoot, "installs.ini")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Default=") {
				return strings.TrimSpace(strings.TrimPrefix(line, "Default="))
			}
		}
	}
	for _, e := range parseProfilesIni(filepath.Join(r.roamingRoot, "profiles.ini")) {
		if e.Default {
			return e.Path
		}
	}
	return ""
}

func parseProfilesIni(path string) []iniProfile {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []iniProfile
	var cur *iniProfile
	// BOM в начале файла: TrimSpace его не снимает (U+FEFF не пробел), и тогда
	// строка "[Profile0]" не распознаётся как секция — первый профиль терялся
	// целиком. Некоторые редакторы и старые версии браузеров пишут BOM.
	text := strings.TrimPrefix(string(data), "\uFEFF")
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if cur != nil && cur.Path != "" {
				out = append(out, *cur)
			}
			cur = nil
			sec := strings.Trim(line, "[]")
			if strings.HasPrefix(sec, "Profile") && !strings.HasPrefix(sec, "ProfileGroups") {
				cur = &iniProfile{}
			}
			continue
		}
		if cur == nil {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.TrimSpace(kv[0]) {
		case "Name":
			cur.Name = strings.TrimSpace(kv[1])
		case "Path":
			cur.Path = strings.TrimSpace(kv[1])
		case "Default":
			cur.Default = strings.TrimSpace(kv[1]) == "1"
		}
	}
	if cur != nil && cur.Path != "" {
		out = append(out, *cur)
	}
	return out
}

func friendlyProfileName(relPath string) string {
	base := filepath.Base(filepath.FromSlash(relPath))
	if i := strings.Index(base, "."); i >= 0 && i+1 < len(base) {
		if looksRandom(base[:i]) {
			return base[i+1:]
		}
	}
	return base
}

func looksRandom(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// ---------- Chromium-like ----------

type localState struct {
	Profile struct {
		InfoCache map[string]struct {
			Name string `json:"name"`
		} `json:"info_cache"`
	} `json:"profile"`
}

func (r *resolved) discoverChromiumLike() []Profile {
	names := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(r.userData, "Local State")); err == nil {
		var ls localState
		if json.Unmarshal(data, &ls) == nil {
			for dir, info := range ls.Profile.InfoCache {
				names[dir] = info.Name
			}
		}
	}

	var out []Profile
	for _, dir := range listDirs(r.userData) {
		if !pathExists(filepath.Join(r.userData, dir, "Preferences")) {
			continue
		}
		name := names[dir]
		if name == "" {
			name = dir
		}
		sources := []SourceItem{{Label: "Profile", Path: filepath.Join(r.userData, dir)}}
		// "Local State" нужен для корректной работы профиля
		if ls := filepath.Join(r.userData, "Local State"); pathExists(ls) {
			sources = append(sources, SourceItem{Label: "Root", Path: ls, IsFile: true})
		}
		out = append(out, Profile{
			ID:          r.spec.Key + "|" + dir,
			Name:        name,
			BrowserKey:  r.spec.Key,
			BrowserName: r.spec.Name,
			Kind:        FamilyChromium,
			RelPath:     dir,
			IsDefault:   dir == "Default",
			IsActive:    dir == "Default",
			Sources:     sources,
		})
	}
	return out
}

// ---------- Сбор групп для интерфейса ----------

func browserGroups() []BrowserGroup {
	var groups []BrowserGroup
	for _, r := range installedBrowsers() {
		g := r.toGroup()
		// Браузер без единого профиля (установлен, но ни разу не запускался)
		// в интерфейсе не показываем — выбирать там нечего.
		if len(g.Profiles) > 0 {
			groups = append(groups, g)
		}
	}
	// Режим «Другой браузер»: папки, добавленные вручную.
	if custom := customGroup(loadSettings().CustomProfileDirs); custom != nil && len(custom.Profiles) > 0 {
		groups = append(groups, *custom)
	}
	return groups
}

// toGroup собирает группу браузера с размерами профилей.
func (r *resolved) toGroup() BrowserGroup {
	profs := r.discoverProfiles()
	fillSizes(profs)
	return BrowserGroup{
		Key:      r.spec.Key,
		Name:     r.spec.Name,
		Family:   string(r.spec.Family),
		Version:  r.browserVersion(),
		Password: r.passwordMode(),
		Profiles: profs,
	}
}

func fillSizes(profs []Profile) {
	for i := range profs {
		var sz int64
		for _, src := range profs[i].Sources {
			if src.IsFile {
				if info, err := os.Stat(src.Path); err == nil {
					sz += info.Size()
				}
			} else {
				sz += dirSize(src.Path)
			}
		}
		profs[i].SizeBytes = sz
		// Профиль без данных: у Firefox-семейства — нет prefs.js/logins.json,
		// у Chromium — нет каталога с данными. Размер 0 сам по себе ещё не значит
		// «пустой» (у Chromium есть Preferences на пару КБ, но данных нет).
		if sz == 0 || !profs[i].hasUserData() {
			profs[i].IsEmpty = true
		}
	}
}

// hasUserData сообщает, есть ли в профиле реальные пользовательские данные.
// Проверяем маркеры обоих семейств: так корректно и для «Другого браузера»,
// куда может попасть папка любого типа.
func (p *Profile) hasUserData() bool {
	for _, src := range p.Sources {
		if src.IsFile {
			continue
		}
		// Firefox-семейство: prefs.js / logins.json.
		if pathExists(filepath.Join(src.Path, "prefs.js")) ||
			pathExists(filepath.Join(src.Path, "logins.json")) {
			return true
		}
		// Chromium-семейство: History / Bookmarks.
		if pathExists(filepath.Join(src.Path, "History")) ||
			pathExists(filepath.Join(src.Path, "Bookmarks")) {
			return true
		}
	}
	return false
}

// customGroup собирает группу «Другой браузер» из папок, добавленных вручную.
// Каждая папка считается отдельным профилем и переносится целиком, как есть.
func customGroup(dirs []string) *BrowserGroup {
	var profs []Profile
	for _, dir := range dirs {
		if !pathExists(dir) {
			continue
		}
		profs = append(profs, Profile{
			ID:          "custom|" + dir,
			Name:        filepath.Base(dir),
			BrowserKey:  "custom",
			BrowserName: T("custom_browser"),
			Kind:        FamilyFirefox,
			RelPath:     dir,
			IsDefault:   len(profs) == 0,
			Sources:     []SourceItem{{Label: "Data", Path: dir}},
		})
	}
	fillSizes(profs)
	return &BrowserGroup{
		Key:      "custom",
		Name:     T("custom_browser"),
		Family:   "custom",
		Password: "files",
		Profiles: profs,
	}
}

func allProfiles() []Profile {
	var out []Profile
	for _, r := range installedBrowsers() {
		out = append(out, r.discoverProfiles()...)
	}
	return out
}

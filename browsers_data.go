package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Семейство браузера определяет, КАК хранятся профили и переносятся ли пароли.
type Family string

const (
	// FamilyFirefox — профили в %APPDATA% (Roaming), список в profiles.ini,
	// пароли и сессии переносятся файлами.
	FamilyFirefox Family = "firefox"
	// FamilyChromium — профили в %LOCALAPPDATA% (Local) как подпапки User Data;
	// пароли/куки шифруются ключом Windows и файлами НЕ переносятся —
	// только через вход в аккаунт (синхронизацию).
	FamilyChromium Family = "chromium"
)

// BrowserSpec — описание браузера. Список браузеров живёт в файле
// browsers.json рядом с .exe (см. loadBrowserSpecs); таблица ниже —
// встроенный набор по умолчанию, который используется, если файла нет.
//
//	Roaming  — путь-кандидат под %APPDATA%      (для Firefox-семейства: папка с profiles.ini)
//	Local    — путь-кандидат под %LOCALAPPDATA% (для Chromium: папка "User Data")
//	Packages — glob под %LOCALAPPDATA%\Packages (для упакованных Store-приложений, напр. Arc)
//
// В одном поле можно указать несколько кандидатов через "|" — берётся первый существующий.
//
//	ProcPath — подстрока полного пути к .exe процесса. Нужна браузерам, чей
//	           исполняемый файл называется так же, как у другого браузера
//	           (Helium использует chrome.exe), — иначе проверка по имени
//	           путает их между собой. Если задан, решает путь, а Proc
//	           используется только как запасной вариант.
type BrowserSpec struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Family   Family   `json:"family"`
	Proc     []string `json:"proc"` // имена процессов без .exe (чтобы проверить, запущен ли браузер)
	ProcPath string   `json:"proc_path,omitempty"`
	Roaming  string   `json:"roaming,omitempty"`
	Local    string   `json:"local,omitempty"`
	Packages string   `json:"packages,omitempty"`
}

// browserTable — фактический список браузеров на этот запуск.
// Заполняется из browsers.json, если файл есть, иначе — встроенным набором.
var browserTable []BrowserSpec

func init() {
	browserTable = loadBrowserSpecs()
}

// browsersConfigPath — путь к внешнему списку браузеров.
// Файл не создаётся автоматически: если пользователь хочет дополнить список,
// он кладёт browsers.json рядом с .exe сам. Программа ничего не пишет в свою папку.
func browsersConfigPath() string {
	return filepath.Join(exeDir(), "browsers.json")
}

// loadBrowserSpecs читает список браузеров: сначала browsers.json рядом с .exe
// (если пользователь его положил), иначе — встроенный набор. Так список
// браузеров — это ДАННЫЕ (файл), а не код.
func loadBrowserSpecs() []BrowserSpec {
	if data, err := os.ReadFile(browsersConfigPath()); err == nil {
		var f struct {
			Browsers []BrowserSpec `json:"browsers"`
		}
		if json.Unmarshal(data, &f) == nil {
			var specs []BrowserSpec
			for _, b := range f.Browsers {
				if b.Key != "" && b.Name != "" && b.Family != "" {
					specs = append(specs, b)
				}
			}
			if len(specs) > 0 {
				return specs
			}
		}
	}
	return builtinBrowsers
}

// builtinBrowsers — встроенный набор по умолчанию. ДОБАВЛЕНИЕ НОВОГО = одна строка.
// Этот же набор записывается в browsers.json как шаблон при первом запуске.
var builtinBrowsers = []BrowserSpec{
	// ---------- Firefox-семейство: пароли переносятся файлами ----------
	{Key: "zen", Name: "Zen", Family: FamilyFirefox, Proc: []string{"zen"},
		Roaming: "zen", Local: "zen"},
	{Key: "firefox", Name: "Firefox", Family: FamilyFirefox, Proc: []string{"firefox"},
		Roaming: "Mozilla/Firefox", Local: "Mozilla/Firefox"},
	{Key: "librewolf", Name: "LibreWolf", Family: FamilyFirefox, Proc: []string{"librewolf"},
		Roaming: "LibreWolf", Local: "LibreWolf"},
	{Key: "waterfox", Name: "Waterfox", Family: FamilyFirefox, Proc: []string{"waterfox"},
		Roaming: "Waterfox", Local: "Waterfox"},
	{Key: "floorp", Name: "Floorp", Family: FamilyFirefox, Proc: []string{"floorp"},
		Roaming: "Floorp", Local: "Floorp"},

	// ---------- Chromium-семейство: пароли только через аккаунт ----------
	{Key: "chrome", Name: "Google Chrome", Family: FamilyChromium, Proc: []string{"chrome"},
		Local: "Google/Chrome/User Data"},
	{Key: "edge", Name: "Microsoft Edge", Family: FamilyChromium, Proc: []string{"msedge"},
		Local: "Microsoft/Edge/User Data"},
	{Key: "brave", Name: "Brave", Family: FamilyChromium, Proc: []string{"brave"},
		Local: "BraveSoftware/Brave-Browser/User Data"},
	{Key: "vivaldi", Name: "Vivaldi", Family: FamilyChromium, Proc: []string{"vivaldi"},
		Local: "Vivaldi/User Data"},
	{Key: "chromium", Name: "Chromium", Family: FamilyChromium, Proc: []string{"chromium"},
		Local: "Chromium/User Data"},
	{Key: "thorium", Name: "Thorium", Family: FamilyChromium, Proc: []string{"thorium"},
		Local: "Thorium/User Data"},
	{Key: "ungoogled", Name: "Ungoogled Chromium", Family: FamilyChromium, Proc: []string{"chromium"},
		Local: "Chromium/User Data|ungoogled-chromium/User Data"},
	{Key: "iron", Name: "SRWare Iron", Family: FamilyChromium, Proc: []string{"iron"},
		Local: "Iron/User Data|SRWare Iron/User Data"},
	// Helium — форк Chromium от imput. Профиль лежит в
	// %LOCALAPPDATA%\imput\Helium\User Data, а сам .exe называется chrome.exe,
	// поэтому запущенность определяется по пути (ProcPath), а не по имени:
	// иначе при открытом Chrome программа считала бы запущенным и Helium.
	{Key: "helium", Name: "Helium", Family: FamilyChromium, Proc: []string{"chrome"},
		ProcPath: "imput/helium/application/chrome.exe",
		Local:    "imput/Helium/User Data"},
	// Arc — упакованное Store-приложение: данные лежат в Packages\<pkg>\LocalCache\Local\...
	{Key: "arc", Name: "Arc", Family: FamilyChromium, Proc: []string{"arc"},
		Packages: "TheBrowserCompany.Arc_*/LocalCache/Local/Arc/User Data"},
}

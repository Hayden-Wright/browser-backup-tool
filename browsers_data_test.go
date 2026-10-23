package main

import (
	"strings"
	"testing"
)

// specByKey ищет браузер во встроенной таблице.
func specByKey(key string) *BrowserSpec {
	for i := range builtinBrowsers {
		if builtinBrowsers[i].Key == key {
			return &builtinBrowsers[i]
		}
	}
	return nil
}

// Helium должен присутствовать и быть Chromium-совместимым: профили лежат
// в User Data, пароли файлами не переносятся.
func TestHeliumIsBuiltIn(t *testing.T) {
	h := specByKey("helium")
	if h == nil {
		t.Fatal("helium missing from builtinBrowsers")
	}
	if h.Family != FamilyChromium {
		t.Errorf("helium family = %q, want %q", h.Family, FamilyChromium)
	}
	if h.Local != "imput/Helium/User Data" {
		t.Errorf("helium Local = %q, want %q", h.Local, "imput/Helium/User Data")
	}
}

// Ключевое свойство: .exe у Helium называется chrome.exe, поэтому запущенность
// обязана определяться по пути, иначе Helium не отличить от Google Chrome.
func TestHeliumDistinguishedByProcessPath(t *testing.T) {
	h := specByKey("helium")
	if h == nil {
		t.Fatal("helium missing from builtinBrowsers")
	}
	if h.ProcPath == "" {
		t.Fatal("helium must set ProcPath: its binary is chrome.exe, same as Chrome")
	}

	// Путь из установки Helium под Windows.
	helium := normalizePath(`C:\Program Files\imput\Helium\Application\chrome.exe`)
	// Путь Google Chrome.
	chrome := normalizePath(`C:\Program Files\Google\Chrome\Application\chrome.exe`)

	want := normalizePath(h.ProcPath)
	if !strings.Contains(helium, want) {
		t.Errorf("helium path %q should match ProcPath %q", helium, want)
	}
	if strings.Contains(chrome, want) {
		t.Errorf("Google Chrome path %q must NOT match Helium ProcPath %q", chrome, want)
	}
}

// Google Chrome не должен получить ProcPath — у него проверка по имени.
func TestChromeUsesProcessName(t *testing.T) {
	c := specByKey("chrome")
	if c == nil {
		t.Fatal("chrome missing from builtinBrowsers")
	}
	if c.ProcPath != "" {
		t.Errorf("chrome ProcPath = %q, want empty (name-based detection)", c.ProcPath)
	}
}

// Ключи браузеров должны быть уникальны, иначе browserByKey вернёт не тот.
func TestBrowserKeysUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range builtinBrowsers {
		if seen[b.Key] {
			t.Errorf("duplicate browser key %q", b.Key)
		}
		seen[b.Key] = true
		if b.Name == "" {
			t.Errorf("browser %q has empty name", b.Key)
		}
	}
}

// Ключевое свойство отката: пустой список путей процессов означает «не смогли
// узнать», а не «ничего не запущено». Иначе Helium при открытом Chrome и
// недоступных путях считался бы закрытым, и бэкап пошёл бы на живом профиле.
func TestProcPathFallsBackWhenPathsUnavailable(t *testing.T) {
	running, decided := runningByProcPath("imput/helium/application/chrome.exe", nil)
	if decided {
		t.Fatal("empty path list must not decide: paths could not be read")
	}
	if running {
		t.Error("running must be false when undecided")
	}
}

// При доступных путях решение принимается строго по пути: Helium и Chrome
// делят chrome.exe, поэтому имя тут не помощник.
func TestProcPathDecidesByPath(t *testing.T) {
	const want = "imput/helium/application/chrome.exe"

	helium := []string{
		normalizePath(`C:\Windows\System32\taskhostw.exe`),
		normalizePath(`C:\Program Files\imput\Helium\Application\chrome.exe`),
	}
	if running, decided := runningByProcPath(want, helium); !decided || !running {
		t.Errorf("helium running=%v decided=%v, want true true", running, decided)
	}

	onlyChrome := []string{
		normalizePath(`C:\Program Files\Google\Chrome\Application\chrome.exe`),
	}
	if running, decided := runningByProcPath(want, onlyChrome); !decided || running {
		t.Errorf("only Chrome open: running=%v decided=%v, want false true", running, decided)
	}
}

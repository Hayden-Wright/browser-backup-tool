package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Портативность: настройки должны лежать РЯДОМ С .exe, а не в %APPDATA%.
// Тест проверяет и путь, и round-trip save→load.
func TestSettingsPortableRoundTrip(t *testing.T) {
	path := settingsPath()
	// Рядом с исполняемым файлом (portable-корень).
	if filepath.Dir(path) != exeDir() {
		t.Fatalf("settings not next to exe: %s vs %s", path, exeDir())
	}
	if filepath.Base(path) != "BrowserBackup-settings.json" {
		t.Fatalf("unexpected settings file name: %s", filepath.Base(path))
	}

	// Не трогаем чужой файл, если он вдруг есть.
	if pathExists(path) {
		t.Skip("settings file already exists; skipping destructive test")
	}
	defer os.Remove(path)

	want := Settings{
		BackupDir:         `D:\SomeDir`,
		Language:          "en",
		CustomProfileDirs: []string{`E:\prof`, `F:\other`},
	}
	saveSettings(want)
	if !pathExists(path) {
		t.Fatalf("saveSettings did not create %s", path)
	}

	got := loadSettings()
	if got.BackupDir != want.BackupDir || got.Language != want.Language {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
	if len(got.CustomProfileDirs) != 2 {
		t.Fatalf("custom dirs lost: %+v", got.CustomProfileDirs)
	}
}

// Настройки не должны зависеть от переменных окружения профиля пользователя
// (APPDATA/LOCALAPPDATA/USERPROFILE) — это и есть «не оставлять следов».
func TestSettingsPathIgnoresAppData(t *testing.T) {
	orig := os.Getenv("APPDATA")
	t.Setenv("APPDATA", filepath.Join(t.TempDir(), "AppData"))
	_ = orig
	if got := settingsPath(); filepath.Dir(got) != exeDir() {
		t.Fatalf("settings path followed APPDATA: %s", got)
	}
}

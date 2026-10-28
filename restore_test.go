package main

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// Восстановление не должно «накладывать» бэкап на существующий профиль:
// после восстановления в целевом профиле остаётся РОВНО содержимое бэкапа,
// а старые файлы уходят в страховочную копию.
func TestRestoreReplacesProfileInsteadOfMerging(t *testing.T) {
	backupDir := t.TempDir()

	// --- «Другой браузер»: путь задан абсолютно, чтобы не зависеть от
	// установленных браузеров на машине с тестами.
	live := filepath.Join(t.TempDir(), "profile")
	writeFile(t, filepath.Join(live, "prefs.js"), []byte("old-settings"))
	writeFile(t, filepath.Join(live, "old-only.txt"), []byte("stale"))

	// Бэкап создаём из отдельной папки.
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "prefs.js"), []byte("new-settings"))
	writeFile(t, filepath.Join(src, "new-only.txt"), []byte("fresh"))

	prof := Profile{
		ID: "custom|" + src, Name: "Work", BrowserKey: "custom",
		BrowserName: "Other", Kind: FamilyFirefox, RelPath: src, IsDefault: true,
		Sources: []SourceItem{{Label: "Data", Path: src}},
	}
	if _, err := doBackup([]Profile{prof}, backupDir, &atomic.Bool{}, nil); err != nil {
		t.Fatalf("doBackup: %v", err)
	}

	mans := listBackups(backupDir)
	if len(mans) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(mans))
	}
	// Возвращаем бэкап «на место»: восстанавливать будем в уже занятую папку.
	mans[0].RelPath = live

	res, err := doRestore(mans, backupDir, &atomic.Bool{}, nil)
	if err != nil {
		t.Fatalf("doRestore: %v", err)
	}
	if res.Count != 1 || len(res.Items) != 1 {
		t.Fatalf("expected 1 restored, got %+v", res)
	}
	// Папка уже была — значит она заменена целиком, а не слита.
	if !res.Items[0].Replaced {
		t.Fatalf("existing folder must be reported as replaced: %+v", res.Items)
	}
	if res.SafetyDir == "" {
		t.Fatal("replaced profile must have a safety copy")
	}

	// В профиле осталось РОВНО содержимое бэкапа.
	if data, _ := os.ReadFile(filepath.Join(live, "prefs.js")); string(data) != "new-settings" {
		t.Fatalf("prefs.js = %q, want the backed-up version", data)
	}
	if !pathExists(filepath.Join(live, "new-only.txt")) {
		t.Fatal("backup file missing after restore")
	}
	if pathExists(filepath.Join(live, "old-only.txt")) {
		t.Fatal("stale file leaked into the restored profile (merge instead of replace)")
	}

	// Старая версия сохранена в страховочной копии.
	safety := filepath.Join(res.SafetyDir, sanitize("custom_Work")+"-1", "Data")
	if data, _ := os.ReadFile(filepath.Join(safety, "old-only.txt")); string(data) != "stale" {
		t.Fatalf("safety copy missing old data: %q", data)
	}
}

// planRestoreTarget: профиль с тем же именем заменяется; профиль с другим
// именем никогда не выбирается «по умолчанию» — вместо этого создаётся новый.
func TestPlanRestoreTargetNeverFallsBackToDefaultProfile(t *testing.T) {
	def := &resolved{spec: BrowserSpec{Key: "chrome", Name: "Chrome", Family: FamilyChromium},
		userData: t.TempDir()}
	current := []Profile{
		{ID: "chrome|Default", Name: "Personal", RelPath: "Default", IsDefault: true},
	}
	man := manifest{BrowserKey: "chrome", BrowserName: "Chrome",
		ProfileName: "Work", RelPath: "Profile 1"}

	got := planRestoreTarget(def, current, man, map[string]bool{})
	if got.replaced {
		t.Fatalf("must not overwrite an unrelated profile: %+v", got)
	}
	if got.profile.RelPath != "Profile 1" {
		t.Fatalf("expected the backup's own path, got %q", got.profile.RelPath)
	}

	// Путь занят другим профилем — берём свободный, но чужой не трогаем.
	os.MkdirAll(filepath.Join(def.userData, "Profile 1"), 0755)
	got = planRestoreTarget(def, current, man, map[string]bool{})
	if got.replaced || got.profile.RelPath == "Profile 1" || got.profile.RelPath == "Default" {
		t.Fatalf("expected a fresh free path, got %+v", got)
	}

	// Тот же профиль (по имени) — заменяем целиком.
	man.ProfileName = "Personal"
	man.RelPath = "Default"
	got = planRestoreTarget(def, current, man, map[string]bool{})
	if !got.replaced || got.profile.RelPath != "Default" {
		t.Fatalf("same-name profile should be replaced: %+v", got)
	}
}

// clearProfileDir удаляет содержимое, но оставляет саму папку.
func TestClearProfileDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), []byte("x"))
	writeFile(t, filepath.Join(dir, "sub", "b.txt"), []byte("y"))
	if err := clearProfileDir(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("dir not cleared: %v %v", entries, err)
	}
	// Несуществующая папка — не ошибка (новый профиль).
	if err := clearProfileDir(filepath.Join(dir, "missing")); err != nil {
		t.Fatalf("missing dir must be fine: %v", err)
	}
}

// Новый профиль Firefox прописывается в profiles.ini ровно один раз,
// даже если секция с этим путём уже есть.
func TestRegisterFirefoxProfileNoDuplicate(t *testing.T) {
	root := t.TempDir()
	ini := filepath.Join(root, "profiles.ini")
	writeFile(t, ini, []byte("[Profile0]\r\nName=Personal\r\nIsRelative=1\r\nPath=Profiles/abc.default\r\n"))

	if err := registerFirefoxProfile(root, "Profiles/new.work", "Work"); err != nil {
		t.Fatal(err)
	}
	entries := parseProfilesIni(ini)
	if len(entries) != 2 {
		t.Fatalf("expected 2 profiles, got %d: %+v", len(entries), entries)
	}

	// Повторная запись того же пути ничего не добавляет.
	if err := registerFirefoxProfile(root, "Profiles/new.work", "Work"); err != nil {
		t.Fatal(err)
	}
	if got := len(parseProfilesIni(ini)); got != 2 {
		t.Fatalf("duplicate section added, now %d profiles", got)
	}
}

// profiles.ini с BOM: первый профиль не должен теряться.
func TestParseProfilesIniWithBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.ini")
	writeFile(t, path, []byte("\uFEFF[Profile0]\nName=First\nPath=Profiles/a\n\n[Profile1]\nName=Second\nPath=Profiles/b\n"))
	entries := parseProfilesIni(path)
	if len(entries) != 2 || entries[0].Name != "First" {
		t.Fatalf("BOM broke parsing: %+v", entries)
	}
}

// Первый запуск: без файла настроек папка бэкапа ещё не выбрана.
func TestNeedsFirstRun(t *testing.T) {
	if (Settings{BackupDir: `D:\BrowserBackup`}).needsFirstRun() != true {
		t.Fatal("default path without explicit choice must trigger the picker")
	}
	if (Settings{BackupDir: `D:\BrowserBackup`, FirstRunDone: true}).needsFirstRun() {
		t.Fatal("FirstRunDone must suppress the picker")
	}
	// Старый формат настроек: путь сохранён через выбор в интерфейсе.
	if (Settings{BackupDir: `D:\BrowserBackup`, BackupDirSet: true}).needsFirstRun() {
		t.Fatal("legacy settings with a chosen dir must not re-ask")
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Журнал при работе из окна живёт во временном каталоге и исчезает при
// выходе: программа не должна оставлять файл browser-backup.log ни в папке
// бэкапа, ни где-либо ещё.
func TestTempLogIsCreatedAndRemoved(t *testing.T) {
	l := &Logger{}
	l.OpenTemp()
	if l.path == "" {
		t.Skip("временный каталог недоступен")
	}
	dir := filepath.Dir(l.path)
	l.Log("hello %s", "world")
	l.Close()

	if pathExists(dir) {
		t.Fatalf("temp log dir %q still exists after Close", dir)
	}
	if l.path != "" {
		t.Fatalf("log path not reset: %q", l.path)
	}

	// Закрывать дважды безопасно.
	l.Close()
}

// Содержимое журнала действительно пишется в файл (иначе файл был бы
// бесполезен для разбора), а после Close(log) — доступно на диске не остаётся.
func TestTempLogWritesLines(t *testing.T) {
	l := &Logger{}
	l.OpenTemp()
	if l.path == "" {
		t.Skip("временный каталог недоступен")
	}
	path := l.path
	l.Log("one")
	l.LogPlain("two")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "one") || !strings.Contains(s, "two") {
		t.Fatalf("log file content unexpected: %q", s)
	}
	l.Close()
}

// doBackup больше не заводит файл журнала в папке бэкапа.
func TestBackupLeavesNoLogFile(t *testing.T) {
	backupDir := t.TempDir()
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "prefs.js"), []byte("x"))

	prof := Profile{
		ID: "t|1", Name: "P", BrowserKey: "t", BrowserName: "T",
		Kind: FamilyFirefox, RelPath: "p1",
		Sources: []SourceItem{{Label: "Roaming", Path: src}},
	}
	if _, err := doBackup([]Profile{prof}, backupDir, &atomic.Bool{}, nil); err != nil {
		t.Fatalf("doBackup: %v", err)
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			t.Fatalf("backup folder contains a log file: %s", e.Name())
		}
	}
}

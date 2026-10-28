package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// Интеграционный тест doBackup: манифест должен содержать сводки, которые
// реально сходятся с записанными файлами (verifyManifest == true), а флаг
// Verified — стоять сразу, без второго прохода по диску.

func TestDoBackupProducesVerifiedManifest(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "prefs.js"), []byte("user_pref(\"x\", 1);"))
	writeFile(t, filepath.Join(src, "sub", "data.bin"), []byte{9, 8, 7, 6, 5})
	writeFile(t, filepath.Join(src, "Cache", "nope"), []byte("skip"))

	backupDir := t.TempDir()
	prof := Profile{
		ID: "test|p1", Name: "TestProfile", BrowserKey: "testkey",
		BrowserName: "TestBrowser", Kind: FamilyFirefox, RelPath: "p1",
		Sources: []SourceItem{{Label: "Roaming", Path: src}},
	}

	res, err := doBackup([]Profile{prof}, backupDir, &atomic.Bool{}, nil)
	if err != nil {
		t.Fatalf("doBackup: %v", err)
	}
	if res.Count != 1 || res.Verified != 1 || res.Broken != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}

	// Читаем манифест с диска.
	entries, _ := os.ReadDir(backupDir)
	var manPath string
	for _, e := range entries {
		if e.IsDir() {
			p := filepath.Join(backupDir, e.Name(), "manifest.json")
			if pathExists(p) {
				manPath = p
			}
		}
	}
	if manPath == "" {
		t.Fatal("manifest.json not found")
	}
	data, _ := os.ReadFile(manPath)
	var man manifest
	if err := json.Unmarshal(data, &man); err != nil {
		t.Fatal(err)
	}
	if man.Verified == nil || !*man.Verified {
		t.Fatalf("manifest not marked verified: %+v", man.Verified)
	}
	if len(man.Digests) != 1 {
		t.Fatalf("expected 1 digest, got %d", len(man.Digests))
	}
	if man.Digests[0].Files != 2 { // prefs.js + sub/data.bin, Cache пропущен
		t.Fatalf("expected 2 files in digest, got %d", man.Digests[0].Files)
	}

	// Сводка должна сойтись с диском.
	if !verifyManifest(backupDir, man) {
		t.Fatal("verifyManifest returned false for a freshly written backup")
	}
}

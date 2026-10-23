package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Тест ключевой инвариант: сводка, посчитанная НА ЛЕТУ при копировании
// (copyTree/copyFile), должна совпадать с той, что даёт отдельный проход
// digestTree по диску. Иначе «проверка целостности» сразу после бэкапа
// начнёт врать (мы убрали повторную проверку именно ради скорости).

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCopyTreeDigestMatchesDigestTree(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "a.txt"), []byte("hello world"))
	writeFile(t, filepath.Join(src, "sub", "b.bin"), []byte{0, 1, 2, 3, 4, 5})
	writeFile(t, filepath.Join(src, "sub", "deep", "c.txt"), []byte("ещё текст"))
	writeFile(t, filepath.Join(src, "empty.txt"), nil)
	// кэш-папка должна быть пропущена при skipCache=true
	writeFile(t, filepath.Join(src, "Cache", "skipme"), []byte("cache"))

	dstRoot := filepath.Join(dst, "tree")
	n, skipped, dg, err := copyTree(src, dstRoot, true, nil, nil)
	if err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped files: %v", skipped)
	}

	// кэш не должен копироваться
	if pathExists(filepath.Join(dstRoot, "Cache", "skipme")) {
		t.Fatal("cache dir was copied despite skipCache=true")
	}

	// эталон — свежий проход по скопированному дереву
	want, err := digestTree(dstRoot, false)
	if err != nil {
		t.Fatalf("digestTree: %v", err)
	}
	dg.Label = ""
	if dg != want {
		t.Fatalf("digest mismatch:\n got %+v\nwant %+v", dg, want)
	}
	if n != want.Bytes {
		t.Fatalf("copied bytes %d != digest bytes %d", n, want.Bytes)
	}
}

func TestCopyFileHashMatchesHashFile(t *testing.T) {
	src := t.TempDir()
	// файл больше буфера копирования (1 MiB), чтобы проверить потоковое хеширование
	data := make([]byte, (1<<20)*2+12345)
	for i := range data {
		data[i] = byte(i * 31)
	}
	srcPath := filepath.Join(src, "big.bin")
	writeFile(t, srcPath, data)

	dstPath := filepath.Join(t.TempDir(), "big.bin")
	buf := make([]byte, 1<<20)
	n, got, err := copyFile(srcPath, dstPath, buf, nil, nil)
	if err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	if n != int64(len(data)) {
		t.Fatalf("copied %d bytes, want %d", n, len(data))
	}

	want, err := hashFile(dstPath)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	if got != want {
		t.Fatalf("on-the-fly hash %s != re-read hash %s", got, want)
	}
}

func TestCopyTreeSingleFileDigest(t *testing.T) {
	// Одиночный файл: сводка строится вручную в doBackup — проверим формулу
	// "base|size|hash" против digestTree(isFile=true).
	src := t.TempDir()
	srcPath := filepath.Join(src, "Prefs")
	writeFile(t, srcPath, []byte("some preferences"))

	dstPath := filepath.Join(t.TempDir(), "Prefs")
	buf := make([]byte, 1<<20)
	n, h, err := copyFile(srcPath, dstPath, buf, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := SourceDigest{
		Files: 1, Bytes: n,
		Hash: hashLines([]string{filepath.Base(srcPath) + "|" + strconv.FormatInt(n, 10) + "|" + h}),
	}
	want, err := digestTree(dstPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("single-file digest mismatch:\n got %+v\nwant %+v", got, want)
	}
}

// Файловое хранилище сайта (storage/default/<site>/fs) — мусор: сайты
// складывают туда произвольные файлы, до 500+ МБ на один сайт. В бэкап не идёт.
// При этом localStorage и IndexedDB обязаны остаться: там состояние
// веб-приложений и входы.
func TestSiteFileStorageSkippedButSiteDataKept(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	site := filepath.Join(src, "storage", "default", "https+++example.com^userContextId=1")
	writeFile(t, filepath.Join(site, "fs", "AB", "bigfile"), []byte("site file blob"))
	writeFile(t, filepath.Join(site, "ls", "data.sqlite"), []byte("localstorage"))
	writeFile(t, filepath.Join(site, "idb", "123.sqlite"), []byte("indexeddb"))

	dstRoot := filepath.Join(dst, "tree")
	if _, _, _, err := copyTree(src, dstRoot, true, nil, nil); err != nil {
		t.Fatalf("copyTree: %v", err)
	}

	if pathExists(filepath.Join(dstRoot, "storage", "default", "https+++example.com^userContextId=1", "fs", "AB", "bigfile")) {
		t.Error("site file storage (fs) was copied, want skipped")
	}
	if !pathExists(filepath.Join(dstRoot, "storage", "default", "https+++example.com^userContextId=1", "ls", "data.sqlite")) {
		t.Error("localStorage (ls) was skipped, want copied")
	}
	if !pathExists(filepath.Join(dstRoot, "storage", "default", "https+++example.com^userContextId=1", "idb", "123.sqlite")) {
		t.Error("IndexedDB (idb) was skipped, want copied")
	}
}

// Папка с именем "fs" вне дерева storage — не файловое хранилище сайта,
// её трогать нельзя: имя слишком общее.
func TestFsOutsideStorageNotSkipped(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "fs", "important.dat"), []byte("keep me"))
	writeFile(t, filepath.Join(src, "extensions", "fs", "addon.dat"), []byte("keep me too"))

	dstRoot := filepath.Join(dst, "tree")
	if _, _, _, err := copyTree(src, dstRoot, true, nil, nil); err != nil {
		t.Fatalf("copyTree: %v", err)
	}

	if !pathExists(filepath.Join(dstRoot, "fs", "important.dat")) {
		t.Error(`top-level "fs" was skipped, want copied`)
	}
	if !pathExists(filepath.Join(dstRoot, "extensions", "fs", "addon.dat")) {
		t.Error(`"fs" inside extensions was skipped, want copied`)
	}
}

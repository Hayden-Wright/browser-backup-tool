package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

var errCancelled = errors.New("cancelled")

// Папки-кэши: не нужны для переноса «личности» браузера, копировать их не будем.
var cacheDirNames = map[string]bool{
	"cache2": true, "startupcache": true, "shader-cache": true, "thumbnails": true,
	"crashes": true, "minidumps": true, "safebrowsing": true, "cache": true,
	"code cache": true, "gpucache": true, "shadercache": true, "grshadercache": true,
	"dawncache": true, "dawngraphitecache": true, "dawnwebgpucache": true,
	"media cache": true, "gpupersistentcache": true, "crashpad": true,
	"browsermetrics": true, "component_crx_cache": true, "extensions_crx_cache": true,
	"meipreload": true, "certificaterevocation": true, "optimization_guide_model_store": true,
	"optimizationhints": true, "service worker": true, "cachestorage": true,
	"scriptcache": true, "jump list icons": true, "jumplisticons": true,
	"session storage": true,
}

// Файлы-замки: их копировать не нужно/нельзя.
var skipFileNames = map[string]bool{
	".parentlock": true, "lock": true, "parent.lock": true,
}

// isSiteFileStorage сообщает, что папка — это файловое хранилище сайта
// (File System API) внутри storage/<тип>/<сайт>/fs.
//
// Почему отдельная проверка, а не просто имя в cacheDirNames: имя "fs"
// слишком общее, и в чужом профиле под ним может лежать что угодно. Здесь
// же важно только одно — мы внутри дерева storage, где fs заводит сам сайт.
//
// Что это за данные. Сайты складывают туда произвольные файлы: кеш игр,
// скачанные ресурсы, офлайн-копии. На реальном профиле Zen это 568 МБ из
// 910 МБ всего бэкапа, причём 524 МБ — один игровой сайт. К настройкам,
// паролям и кукам отношения не имеет, поэтому в бэкап не идёт.
// IndexedDB (idb) и localStorage (ls) остаются: там как раз состояние
// веб-приложений и входы.
func isSiteFileStorage(dirPath string) bool {
	if !strings.EqualFold(filepath.Base(dirPath), "fs") {
		return false
	}
	p := normalizePath(dirPath)
	return strings.Contains(p, "/storage/default/") || strings.Contains(p, "/storage/permanent/")
}

type fileEntry struct {
	path string
	size int64
}

// collectFiles обходит дерево и собирает список файлов для копирования.
func collectFiles(root string, skipCache bool) ([]fileEntry, int64) {
	var files []fileEntry
	var total int64

	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // недоступное пропускаем, не роняем весь обход
		}
		name := strings.ToLower(info.Name())

		if info.IsDir() {
			if skipCache && (cacheDirNames[name] || isSiteFileStorage(p)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil // символические ссылки и спецфайлы не копируем
		}
		if skipFileNames[name] {
			return nil
		}
		files = append(files, fileEntry{path: p, size: info.Size()})
		total += info.Size()
		return nil
	})
	return files, total
}

// copyTree копирует дерево root в dst. Возвращает (скопировано, пропущено, сводка, ошибка).
// Хеш каждого файла считается НА ЛЕТУ при копировании, поэтому отдельный проход
// для проверки целостности не нужен. onBytes вызывается по мере копирования.
func copyTree(root, dst string, skipCache bool, cancel *atomic.Bool, onBytes func(int64)) (int64, []string, SourceDigest, error) {
	files, _ := collectFiles(root, skipCache)
	var copied int64
	var skipped []string
	var lines []string
	var d SourceDigest
	buf := make([]byte, 1<<20) // 1 MiB

	for _, f := range files {
		if cancel != nil && cancel.Load() {
			return copied, skipped, d, errCancelled
		}
		rel, err := filepath.Rel(root, f.path)
		if err != nil {
			continue
		}
		target := filepath.Join(dst, rel)

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			skipped = append(skipped, f.path)
			continue
		}
		n, h, err := copyFile(f.path, target, buf, cancel, onBytes)
		copied += n
		if err != nil {
			if errors.Is(err, errCancelled) {
				return copied, skipped, d, errCancelled
			}
			// занятый или недоступный файл — пропускаем, но фиксируем
			skipped = append(skipped, f.path)
			continue
		}
		// сводка — по тому, что реально записано (dst), с хешем из копирования
		lines = append(lines, fmt.Sprintf("%s|%d|%s", filepath.ToSlash(rel), n, h))
		d.Files++
		d.Bytes += n
	}
	d.Hash = hashLines(lines)
	return copied, skipped, d, nil
}

// copyFile копирует один файл и заодно возвращает sha256 записанного
// содержимого — чтобы не читать файл второй раз ради проверки целостности.
// onBytes вызывается по мере копирования (для прогресса).
func copyFile(src, dst string, buf []byte, cancel *atomic.Bool, onBytes func(int64)) (int64, string, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, "", err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return 0, "", err
	}

	h := sha256.New()
	var written int64
	for {
		if cancel != nil && cancel.Load() {
			out.Close()
			return written, "", errCancelled
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return written, "", werr
			}
			h.Write(buf[:n])
			written += int64(n)
			if onBytes != nil {
				onBytes(int64(n))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return written, "", rerr
		}
	}
	if err := out.Close(); err != nil {
		return written, "", err
	}
	// сохраняем время изменения
	if info, err := os.Stat(src); err == nil {
		os.Chtimes(dst, time.Now(), info.ModTime())
	}
	return written, hex.EncodeToString(h.Sum(nil)), nil
}

// hashLines — общий хеш набора строк "relpath|size|filehash" (как в digestTree).
func hashLines(lines []string) string {
	sorted := append([]string(nil), lines...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}

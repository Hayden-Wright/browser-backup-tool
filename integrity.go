package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// SourceDigest — сводка по одному источнику (Roaming/Local/Profile) для проверки целостности.
type SourceDigest struct {
	Label string `json:"label"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
	Hash  string `json:"hash"` // sha256 от отсортированных строк "relpath|size|filehash"
}

// digestTree вычисляет сводку по дереву (или одиночному файлу).
func digestTree(root string, isFile bool) (SourceDigest, error) {
	d := SourceDigest{}
	var lines []string

	add := func(rel, path string, size int64) error {
		h, err := hashFile(path)
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s|%d|%s", rel, size, h))
		d.Files++
		d.Bytes += size
		return nil
	}

	if isFile {
		info, err := os.Stat(root)
		if err != nil {
			return d, err
		}
		if err := add(filepath.Base(root), root, info.Size()); err != nil {
			return d, err
		}
	} else {
		err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil || !info.Mode().IsRegular() {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			return add(filepath.ToSlash(rel), p, info.Size())
		})
		if err != nil {
			return d, err
		}
	}

	sort.Strings(lines)
	d.Hash = hashLines(lines)
	return d, nil
}

// verifyDigest сверяет сохранённую сводку с тем, что реально лежит на диске.
func verifyDigest(root string, isFile bool, want SourceDigest) (bool, string) {
	got, err := digestTree(root, isFile)
	if err != nil {
		return false, fmt.Sprintf("не удалось прочитать: %v", err)
	}
	if got.Files != want.Files {
		return false, fmt.Sprintf("файлов %d вместо %d", got.Files, want.Files)
	}
	if got.Bytes != want.Bytes {
		return false, fmt.Sprintf("размер %s вместо %s", humanSize(got.Bytes), humanSize(want.Bytes))
	}
	if got.Hash != want.Hash {
		return false, "содержимое файлов отличается"
	}
	return true, ""
}

// hashFile считает sha256 файла.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

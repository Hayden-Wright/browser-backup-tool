package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// RestoreResult — итог восстановления.
type RestoreResult struct {
	Count       int
	Bytes       int64
	Skipped     int
	SafetyDir   string
	NotRun      []string // браузеры, которые нужно запустить один раз
	NotInBackup []string
	Broken      []string // бэкапы, не прошедшие проверку целостности
}

// doRestore восстанавливает выбранные бэкапы (по manifest) на текущую систему.
func doRestore(backups []manifest, backupDir string, cancel *atomic.Bool,
	progress func(done, total int64, name string)) (*RestoreResult, error) {

	res := &RestoreResult{}
	log.SetFile(backupDir)

	stamp := timestamp()
	safetyBase := filepath.Join(backupDir, "_safety", stamp)

	// Оценка объёма.
	var total int64
	for _, man := range backups {
		src := backupSourceDir(backupDir, man)
		total += dirSize(src)
	}

	var done int64
	buf := make([]byte, 1<<20)
	usedTargets := map[string]bool{} // ID профилей, уже занятых в этом запуске

	for _, man := range backups {
		if cancel.Load() {
			return res, errCancelled
		}

		// Перед восстановлением проверяем целостность — но только если бэкап
		// не был проверен при создании. Иначе это лишнее полное хеширование
		// всего профиля (десятки секунд на больших данных).
		if man.Verified == nil || !*man.Verified {
			if progress != nil {
				progress(done, total, T("st_verifying"))
			}
			if !verifyManifest(backupDir, man) {
				res.Broken = append(res.Broken, man.BrowserName+" — "+man.ProfileName)
				log.Log("Integrity check FAILED, skipping: %s — %s", man.BrowserName, man.ProfileName)
				continue
			}
		}

		def := browserByKey(man.BrowserKey)
		var target Profile
		if man.BrowserKey == "custom" {
			// Режим «Другой браузер»: восстанавливаем строго по исходному
			// абсолютному пути, без поиска профилей.
			def = &resolved{spec: BrowserSpec{Key: "custom", Name: T("custom_browser"), Family: FamilyFirefox}}
			target = Profile{
				ID: "custom|" + man.RelPath, Name: man.ProfileName,
				BrowserKey: "custom", BrowserName: man.BrowserName,
				Kind: FamilyFirefox, RelPath: man.RelPath, IsDefault: true,
				Sources: []SourceItem{{Label: "Data", Path: man.RelPath}},
			}
		} else {
			if def == nil {
				res.NotInBackup = append(res.NotInBackup, man.BrowserName)
				log.Log("Warning: browser not installed: %s", man.BrowserName)
				continue
			}

			current := def.discoverProfiles()
			if len(current) == 0 {
				// Браузер не установлен или ещё не создал профиль —
				// в обоих случаях нужно один раз запустить и закрыть его.
				res.NotRun = append(res.NotRun, def.spec.Name)
				log.Log("Warning: no profile yet for %s (install and launch it once)", def.spec.Name)
				continue
			}

			target = pickTargetProfile(current, man, usedTargets)
			usedTargets[target.ID] = true
		}
		srcRoot := backupSourceDir(backupDir, man)

		log.Log("Restoring %s — %s", man.BrowserName, man.ProfileName)
		if progress != nil {
			progress(done, total, def.spec.Name+" — "+man.ProfileName)
		}

		// 1) Страховочная копия текущего профиля.
		if progress != nil {
			progress(done, total, T("st_safety"))
		}
		safetyDst := filepath.Join(safetyBase, sanitize(man.BrowserKey+"_"+man.ProfileName))
		copyCurrentForSafety(target, safetyDst)
		res.SafetyDir = safetyBase
		log.Log("Safety copy: %s", safetyDst)

		// 2) Копируем содержимое бэкапа в цель.
		for _, srcItem := range sourceItems(srcRoot) {
			if cancel.Load() {
				return res, errCancelled
			}
			dst, ok := mapSourceToTarget(def, target, srcItem.Label)
			if !ok {
				continue
			}
			if srcItem.IsFile {
				os.MkdirAll(filepath.Dir(dst), 0755)
				n, _, err := copyFile(srcItem.Path, dst, buf, cancel, func(k int64) {
					done += k
					if progress != nil {
						progress(done, total, def.spec.Name+" — "+man.ProfileName)
					}
				})
				res.Bytes += n
				if err != nil {
					if err == errCancelled {
						return res, errCancelled
					}
					res.Skipped++
					log.Log("Skipped locked file: %s", dst)
				}
				continue
			}
			n, skipped, _, err := copyTree(srcItem.Path, dst, false, cancel, func(k int64) {
				done += k
				if progress != nil {
					progress(done, total, def.spec.Name+" — "+man.ProfileName)
				}
			})
			res.Bytes += n
			res.Skipped += len(skipped)
			for _, sk := range skipped {
				log.Log("Skipped locked file: %s", sk)
			}
			if err != nil {
				if err == errCancelled {
					return res, errCancelled
				}
				return res, err
			}
		}

		res.Count++
		log.Log("Copied %s", humanSize(res.Bytes))
	}

	return res, nil
}

// backupSourceDir — папка конкретного бэкапа по manifest.
func backupSourceDir(backupDir string, man manifest) string {
	if man.Folder != "" {
		p := filepath.Join(backupDir, man.Folder)
		if pathExists(p) {
			return p
		}
	}
	return findBackupDir(backupDir, man)
}

func readManifest(path string) (manifest, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, false
	}
	var man manifest
	if err := json.Unmarshal(data, &man); err != nil {
		return manifest{}, false
	}
	return man, true
}

// pickTargetProfile выбирает профиль на новой системе, в который будем писать.
// used — уже занятые профили (чтобы два бэкапа не «схлопнулись» в один).
func pickTargetProfile(current []Profile, man manifest, used map[string]bool) Profile {
	// 1) тот же путь, что и в бэкапе (совпало, если профиль не пересоздавали)
	for _, p := range current {
		if !used[p.ID] && p.RelPath == man.RelPath {
			return p
		}
	}
	// 2) совпадение по имени — при новой установке id меняется, а имя остаётся
	for _, p := range current {
		if !used[p.ID] && strings.EqualFold(p.Name, man.ProfileName) {
			return p
		}
	}
	// 3) профиль по умолчанию
	for _, p := range current {
		if !used[p.ID] && p.IsDefault {
			return p
		}
	}
	// 4) первый свободный
	for _, p := range current {
		if !used[p.ID] {
			return p
		}
	}
	// 5) всё занято — возвращаем по умолчанию, как крайний случай
	for _, p := range current {
		if p.IsDefault {
			return p
		}
	}
	return current[0]
}

// sourceItems перечисляет источники внутри папки бэкапа.
func sourceItems(dir string) []SourceItem {
	var out []SourceItem
	for _, sub := range listDirs(dir) {
		if sub == "_safety" {
			continue
		}
		p := filepath.Join(dir, sub)
		out = append(out, SourceItem{Label: sub, Path: p})
	}
	// одиночные файлы на верхнем уровне (на случай Root-файлов)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == "manifest.json" || e.Name() == "ПРОЧТИ_МЕНЯ.txt" {
			continue
		}
		out = append(out, SourceItem{Label: e.Name(), Path: filepath.Join(dir, e.Name()), IsFile: true})
	}
	return out
}

// mapSourceToTarget сопоставляет источник бэкапа с путём на новой системе.
func mapSourceToTarget(def *resolved, target Profile, label string) (string, bool) {
	switch label {
	case "Roaming":
		if def.spec.Family == FamilyFirefox && def.roamingRoot != "" {
			return filepath.Join(def.roamingRoot, filepath.FromSlash(target.RelPath)), true
		}
	case "Local":
		if def.spec.Family == FamilyFirefox && def.localRoot != "" {
			return filepath.Join(def.localRoot, filepath.FromSlash(target.RelPath)), true
		}
	case "Profile":
		if def.spec.Family == FamilyChromium && def.userData != "" {
			return filepath.Join(def.userData, target.RelPath), true
		}
	case "Root":
		if def.spec.Family == FamilyChromium && def.userData != "" {
			return def.userData, true
		}
	case "Data":
		// Режим «Другой браузер»: возвращаем по исходному абсолютному пути.
		if def.spec.Key == "custom" && target.RelPath != "" {
			return target.RelPath, true
		}
	}
	return "", false
}

// copyCurrentForSafety сохраняет текущие данные профиля перед заменой.
func copyCurrentForSafety(target Profile, dstRoot string) {
	buf := make([]byte, 1<<20)
	for _, src := range target.Sources {
		if src.IsFile {
			dst := filepath.Join(dstRoot, src.Label, filepath.Base(src.Path))
			os.MkdirAll(filepath.Dir(dst), 0755)
			copyFile(src.Path, dst, buf, nil, nil)
			continue
		}
		dst := filepath.Join(dstRoot, src.Label)
		copyTree(src.Path, dst, false, nil, nil)
	}
}

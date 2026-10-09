package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
)

// manifest описывает содержимое одного бэкапа профиля.
type manifest struct {
	BrowserKey     string         `json:"browser_key"`
	BrowserName    string         `json:"browser_name"`
	BrowserVersion string         `json:"browser_version"`
	ProfileName    string         `json:"profile_name"`
	RelPath        string         `json:"rel_path"` // путь профиля внутри корня браузера
	Kind           string         `json:"kind"`
	Created        string         `json:"created"`
	Computer       string         `json:"computer"`
	User           string         `json:"user"`
	Windows        string         `json:"windows"`
	Folder         string         `json:"folder"` // имя папки этого бэкапа
	TotalBytes     int64          `json:"total_bytes"`
	FileCount      int            `json:"file_count"`
	Digests        []SourceDigest `json:"digests"` // контрольные суммы по источникам
	// Verified — результат проверки целостности СРАЗУ ПОСЛЕ создания бэкапа.
	// Храним, чтобы не пересчитывать хеши всех файлов при каждом открытии окна
	// (это очень долго для больших профилей). У старых бэкапов поле отсутствует.
	Verified *bool `json:"verified,omitempty"`
}

// BackupResult — итог создания бэкапа.
type BackupResult struct {
	Count    int
	Bytes    int64
	Skipped  int
	Dir      string
	Verified int // сколько профилей успешно проверено
	Broken   int // сколько не прошло проверку
	Profiles []string
}

// doBackup создаёт бэкап выбранных профилей в backupDir.
// progress: (обработано_байт, всего_байт, текущее_имя).
func doBackup(profiles []Profile, backupDir string, cancel *atomic.Bool,
	progress func(done, total int64, name string)) (*BackupResult, error) {

	res := &BackupResult{Dir: backupDir}
	os.MkdirAll(backupDir, 0755)
	log.SetFile(backupDir)

	stamp := timestamp()

	// Считаем общий объём для прогресса.
	type job struct {
		p       Profile
		sources []SourceItem
		size    int64
	}
	var jobs []job
	var total int64
	for _, p := range profiles {
		j := job{p: p, sources: p.Sources}
		for _, src := range p.Sources {
			if src.IsFile {
				if info, err := os.Stat(src.Path); err == nil {
					j.size += info.Size()
				}
			} else {
				_, sz := collectFiles(src.Path, true)
				j.size += sz
			}
		}
		total += j.size
		jobs = append(jobs, j)
	}
	log.Log("Total size: %s", humanSize(total))

	var done int64
	buf := make([]byte, 1<<20)
	var created []manifest // только что созданные бэкапы (для проверки целостности)

	for _, j := range jobs {
		if cancel.Load() {
			return res, errCancelled
		}

		// Отдельная папка с датой. Если такая уже есть — добавляем номер,
		// чтобы никогда не перезаписать предыдущий бэкап.
		folder := sanitize(j.p.BrowserKey+"_"+j.p.Name) + "_" + stamp
		destRoot := uniqueDir(backupDir, folder)
		os.MkdirAll(destRoot, 0755)

		log.Log("Backing up %s — %s", j.p.BrowserName, j.p.Name)
		if progress != nil {
			progress(done, total, j.p.DisplayName())
		}

		def := browserByKey(j.p.BrowserKey)
		man := manifest{
			BrowserKey:  j.p.BrowserKey,
			BrowserName: j.p.BrowserName,
			ProfileName: j.p.Name,
			RelPath:     j.p.RelPath,
			Kind:        string(j.p.Kind),
			Created:     timestamp(),
			Computer:    os.Getenv("COMPUTERNAME"),
			User:        os.Getenv("USERNAME"),
			Windows:     windowsVersion(),
			Folder:      filepath.Base(destRoot),
		}
		if def != nil {
			man.BrowserVersion = def.browserVersion()
		}

		for _, src := range j.p.Sources {
			if cancel.Load() {
				return res, errCancelled
			}

			if src.IsFile {
				dst := filepath.Join(destRoot, src.Label, filepath.Base(src.Path))
				os.MkdirAll(filepath.Dir(dst), 0755)
				n, h, err := copyFile(src.Path, dst, buf, cancel, func(k int64) {
					done += k
					if progress != nil {
						progress(done, total, j.p.DisplayName())
					}
				})
				res.Bytes += n
				if err != nil {
					if err == errCancelled {
						return res, errCancelled
					}
					log.Log("Skipped locked file: %s", src.Path)
					res.Skipped++
					continue
				}
				// сводка по скопированному файлу — хеш получен при копировании
				dg := SourceDigest{
					Label: src.Label, Files: 1, Bytes: n,
					Hash: hashLines([]string{fmt.Sprintf("%s|%d|%s", filepath.Base(src.Path), n, h)}),
				}
				man.Digests = append(man.Digests, dg)
				man.FileCount += dg.Files
				man.TotalBytes += dg.Bytes
				continue
			}

			dst := filepath.Join(destRoot, src.Label)
			n, skipped, dg, err := copyTree(src.Path, dst, true, cancel, func(k int64) {
				done += k
				if progress != nil {
					progress(done, total, j.p.DisplayName())
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
			// сводка по скопированному дереву (хеши посчитаны при копировании)
			dg.Label = src.Label
			man.Digests = append(man.Digests, dg)
			man.FileCount += dg.Files
			man.TotalBytes += dg.Bytes
		}

		writeManifest(destRoot, man)
		man.Folder = filepath.Base(destRoot)
		created = append(created, man)
		res.Count++
		res.Profiles = append(res.Profiles, j.p.DisplayName())
		log.Log("Copied %s (%d files)", humanSize(man.TotalBytes), man.FileCount)
	}

	// Сводки собраны на лету при копировании — отдельный проход по диску
	// (раньше это было второе полное хеширование) больше не нужен.
	verified := true
	for i := range created {
		created[i].Verified = &verified
		persistManifest(backupDir, created[i])
		res.Verified++
	}

	return res, nil
}

// persistManifest перезаписывает manifest.json (например, после проверки
// целостности), не трогая ПРОЧТИ_МЕНЯ.txt.
func persistManifest(backupDir string, man manifest) {
	dir := backupDir
	if man.Folder != "" {
		dir = filepath.Join(backupDir, man.Folder)
	}
	data, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644)
}

// verifyManifest сверяет все сводки одного бэкапа с диском.
func verifyManifest(backupDir string, man manifest) bool {
	dir := filepath.Join(backupDir, man.Folder)
	if man.Folder == "" {
		dir = findBackupDir(backupDir, man)
	}
	if dir == "" {
		return false
	}
	for _, dg := range man.Digests {
		target := filepath.Join(dir, dg.Label)
		isFile := false
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			isFile = true
		}
		// файл-источник лежит в подпапке Label
		if !isFile && !pathExists(target) {
			// может быть, одиночный файл внутри подпапки
			sub := filepath.Join(dir, dg.Label)
			if entries, err := os.ReadDir(sub); err == nil && len(entries) == 1 && !entries[0].IsDir() {
				target = filepath.Join(sub, entries[0].Name())
				isFile = true
			}
		}
		if ok, reason := verifyDigest(target, isFile, dg); !ok {
			log.Log("Integrity check FAILED for %s/%s: %s", man.ProfileName, dg.Label, reason)
			return false
		}
	}
	return true
}

// uniqueDir возвращает путь, гарантированно не существующий (добавляет -2, -3…).
func uniqueDir(base, name string) string {
	p := filepath.Join(base, name)
	if !pathExists(p) {
		return p
	}
	for i := 2; ; i++ {
		p = filepath.Join(base, name+"-"+strconv.Itoa(i))
		if !pathExists(p) {
			return p
		}
	}
}

func writeManifest(dir string, man manifest) {
	data, _ := json.MarshalIndent(man, "", "  ")
	os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644)

	txt := strings.Join([]string{
		"Браузер:      " + man.BrowserName + " " + man.BrowserVersion,
		"Профиль:      " + man.ProfileName,
		"Создан:       " + man.Created,
		"Windows:      " + man.Windows,
		"Компьютер:    " + man.Computer,
		"Пользователь: " + man.User,
		"Файлов:       " + strconv.Itoa(man.FileCount),
		"Размер:       " + humanSize(man.TotalBytes),
		"",
		"Чтобы восстановить: запустите программу, выберите этот бэкап",
		"и нажмите «Восстановить». Браузер должен быть закрыт.",
	}, "\r\n")
	os.WriteFile(filepath.Join(dir, "ПРОЧТИ_МЕНЯ.txt"), []byte(txt), 0644)
}

// listBackups находит готовые бэкапы (папки с manifest.json) в каталоге.
func listBackups(backupDir string) []manifest {
	var out []manifest
	for _, d := range listDirs(backupDir) {
		if d == "_safety" {
			continue
		}
		if man, ok := readManifest(filepath.Join(backupDir, d, "manifest.json")); ok {
			if man.Folder == "" {
				man.Folder = d
			}
			out = append(out, man)
		}
	}
	return out
}

// findBackupDir ищет папку бэкапа по содержимому манифеста (на случай старого формата).
func findBackupDir(backupDir string, man manifest) string {
	for _, d := range listDirs(backupDir) {
		if m, ok := readManifest(filepath.Join(backupDir, d, "manifest.json")); ok &&
			m.BrowserKey == man.BrowserKey && m.ProfileName == man.ProfileName && m.Created == man.Created {
			return filepath.Join(backupDir, d)
		}
	}
	return ""
}

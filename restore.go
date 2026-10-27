package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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
	Broken      []string      // бэкапы, не прошедшие проверку целостности
	Items       []RestoreItem // что именно легло и куда
}

// RestoreItem — один восстановленный профиль.
type RestoreItem struct {
	Browser  string `json:"browser"`
	Name     string `json:"name"`
	Replaced bool   `json:"replaced"` // true — заменён существующий профиль
	Path     string `json:"path"`     // путь на диске
}

// doRestore восстанавливает выбранные бэкапы (по manifest) на текущую систему.
//
// Главное правило: НИКОГДА не смешивать бэкап с существующим профилем.
// Раньше содержимое бэкапа копировалось поверх текущего профиля файл за
// файлом — оставались и старые файлы, и новые, и профиль превращался в
// смесь двух (см. issue: «один профиль наложился на другой»). Теперь профиль-
// приёмник либо ЗАМЕНЯЕТСЯ целиком (старый сначала копируется в _safety),
// либо создаётся заново, если подходящего профиля нет.
func doRestore(backups []manifest, backupDir string, cancel *atomic.Bool,
	progress func(done, total int64, name string)) (*RestoreResult, error) {

	res := &RestoreResult{}
	log.SetFile(backupDir)

	stamp := timestamp()
	safetyBase := filepath.Join(backupDir, "_safety", stamp)

	// Оценка объёма.
	var total int64
	for _, man := range backups {
		total += dirSize(backupSourceDir(backupDir, man))
	}

	var done int64
	buf := make([]byte, 1<<20)
	used := map[string]bool{} // ID профилей, уже занятых в этом запуске

	for i, man := range backups {
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

		var (
			def    *resolved
			target restoreTarget
		)

		if man.BrowserKey == "custom" {
			// Режим «Другой браузер»: путь задан абсолютно. Если папка уже
			// есть — не трогаем её, а восстанавливаем рядом.
			def = &resolved{spec: BrowserSpec{Key: "custom", Name: T("custom_browser"), Family: FamilyFirefox}}
			target = planCustomRestore(man, stamp)
		} else {
			def = browserByKey(man.BrowserKey)
			if def == nil {
				res.NotInBackup = append(res.NotInBackup, man.BrowserName)
				log.Log("Warning: browser unknown: %s", man.BrowserName)
				continue
			}
			if !def.isInstalled() {
				// Браузера нет или он ещё ни разу не запускался: создать папку
				// профиля нам негде. Пользователь запускает и повторяет.
				res.NotRun = append(res.NotRun, def.spec.Name)
				log.Log("Warning: no profile yet for %s (install and launch it once)", def.spec.Name)
				continue
			}
			current := def.discoverProfiles()
			target = planRestoreTarget(def, current, man, used)
			if target.profile.ID != "" {
				used[target.profile.ID] = true
			}
		}

		srcRoot := backupSourceDir(backupDir, man)
		dispName := def.spec.Name + " — " + man.ProfileName

		log.Log("Restoring %s — %s → %s (replaced=%v)",
			man.BrowserName, man.ProfileName, target.profile.RelPath, target.replaced)
		if progress != nil {
			progress(done, total, dispName)
		}

		// 1) Страховочная копия текущего профиля — ДО любых изменений.
		if target.replaced {
			if progress != nil {
				progress(done, total, T("st_safety"))
			}
			safetyDst := filepath.Join(safetyBase,
				sanitize(man.BrowserKey+"_"+man.ProfileName)+"-"+strconv.Itoa(i+1))
			copyCurrentForSafety(target.profile, safetyDst)
			res.SafetyDir = safetyBase
			log.Log("Safety copy: %s", safetyDst)
		}

		// 2) Определяем, куда ляжет каждый источник, и ОЧИЩАЕМ приёмник.
		//    Именно очистка отличает восстановление от «наложения»: после неё
		//    в профиле остаётся ровно то, что было в бэкапе.
		type job struct {
			src, dst string
			isFile   bool
		}
		var jobs []job
		for _, srcItem := range sourceItems(srcRoot) {
			dst, ok := mapSourceToTarget(def, target.profile, srcItem.Label)
			if !ok {
				continue
			}
			jobs = append(jobs, job{src: srcItem.Path, dst: dst, isFile: srcItem.IsFile})
		}
		for _, j := range jobs {
			if j.isFile {
				continue
			}
			if err := clearProfileDir(j.dst); err != nil {
				log.Log("Warning: cannot clear %s: %v", j.dst, err)
			}
		}

		// 3) Копируем содержимое бэкапа в цель.
		for _, j := range jobs {
			if cancel.Load() {
				return res, errCancelled
			}
			if j.isFile {
				os.MkdirAll(filepath.Dir(j.dst), 0755)
				n, _, err := copyFile(j.src, j.dst, buf, cancel, func(k int64) {
					done += k
					if progress != nil {
						progress(done, total, dispName)
					}
				})
				res.Bytes += n
				if err != nil {
					if err == errCancelled {
						return res, errCancelled
					}
					res.Skipped++
					log.Log("Skipped locked file: %s", j.dst)
				}
				continue
			}
			n, skipped, _, err := copyTree(j.src, j.dst, false, cancel, func(k int64) {
				done += k
				if progress != nil {
					progress(done, total, dispName)
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

		// 4) Новый профиль прописываем в конфигурации браузера, иначе данные
		//    лежат на диске, но в списке профилей его нет. Заменённый профиль
		//    уже там — повторная запись создала бы дубликат.
		if man.BrowserKey != "custom" && !target.replaced {
			if err := registerNewProfile(def, target.profile.RelPath, target.name); err != nil {
				log.Log("Warning: cannot register profile %s: %v", target.profile.RelPath, err)
			}
		}

		res.Items = append(res.Items, RestoreItem{
			Browser:  man.BrowserName,
			Name:     man.ProfileName,
			Replaced: target.replaced,
			Path:     target.profile.RelPath,
		})
		res.Count++
		log.Log("Copied %s", humanSize(res.Bytes))
	}

	return res, nil
}

// restoreTarget — выбранный профиль-приёмник.
type restoreTarget struct {
	profile  Profile // цель: RelPath (и Sources — для страховочной копии)
	replaced bool    // true — пишем поверх существующего профиля
	name     string  // имя профиля для интерфейса/журнала
}

// planRestoreTarget решает, в какой профиль писать бэкап.
//
// Порядок поиска (важно: раньше был опасный откат «в профиль по умолчанию»,
// из-за которого данные одного профиля попадали в совершенно другой):
//  1. профиль с тем же ИМЕНЕМ — это тот же профиль, даже если папка другая;
//  2. профиль по тому же пути;
//  3. ничего не подошло — создаём НОВЫЙ профиль, существующие не трогаем.
func planRestoreTarget(def *resolved, current []Profile, man manifest, used map[string]bool) restoreTarget {
	var byName []Profile
	for _, p := range current {
		if used[p.ID] {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(p.Name), strings.TrimSpace(man.ProfileName)) {
			byName = append(byName, p)
		}
	}
	if len(byName) == 1 {
		return restoreTarget{profile: byName[0], replaced: true, name: byName[0].Name}
	}
	if len(byName) > 1 {
		for _, p := range byName {
			if sameRelPath(p.RelPath, man.RelPath) {
				return restoreTarget{profile: p, replaced: true, name: p.Name}
			}
		}
		return restoreTarget{profile: byName[0], replaced: true, name: byName[0].Name}
	}

	for _, p := range current {
		if used[p.ID] {
			continue
		}
		if sameRelPath(p.RelPath, man.RelPath) {
			return restoreTarget{profile: p, replaced: true, name: p.Name}
		}
	}

	// Новый профиль на пути из бэкапа (или ближайшем свободном).
	rel := man.RelPath
	if rel == "" || relPathTaken(def, current, rel) {
		rel = freeRelPath(def, current)
	}
	return restoreTarget{profile: Profile{RelPath: rel}, name: man.ProfileName}
}

// planCustomRestore выбирает путь для режима «Другой браузер».
//
// Восстанавливаем строго в исходную папку. Если она уже есть, она заменяется
// целиком, а её содержимое уходит в страховочную копию — ровно так же, как
// с профилем браузера. Дубликат «(restored …)» рядом с папкой не создаём:
// пользователь выбрал конкретный путь и ожидает данные именно там.
func planCustomRestore(man manifest, stamp string) restoreTarget {
	rel := man.RelPath
	if rel == "" {
		rel = filepath.Join(os.Getenv("USERPROFILE"), "BrowserBackup-restored-"+stamp)
	}
	return restoreTarget{
		profile: Profile{
			ID: "custom|" + rel, Name: man.ProfileName, BrowserKey: "custom",
			BrowserName: man.BrowserName, Kind: FamilyFirefox, RelPath: rel,
			IsDefault: true,
			Sources:   []SourceItem{{Label: "Data", Path: rel}},
		},
		replaced: pathExists(rel),
		name:     man.ProfileName,
	}
}

// clearProfileDir освобождает папку профиля: удаляет её содержимое, но не
// саму папку (Chromium/Firefox ждут, что каталог профиля существует).
// Отсутствующая папка — не ошибка: это новый профиль.
func clearProfileDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// sameRelPath сравнивает пути профилей без учёта регистра и вида слэшей.
func sameRelPath(a, b string) bool {
	na := strings.ReplaceAll(strings.Trim(strings.ToLower(a), `\/`), `\`, "/")
	nb := strings.ReplaceAll(strings.Trim(strings.ToLower(b), `\/`), `\`, "/")
	return na != "" && na == nb
}

// relPathTaken сообщает, занят ли путь профиля — профилем из списка или
// папкой на диске.
func relPathTaken(def *resolved, current []Profile, rel string) bool {
	for _, p := range current {
		if sameRelPath(p.RelPath, rel) {
			return true
		}
	}
	for _, root := range profileRoots(def) {
		if pathExists(filepath.Join(root, filepath.FromSlash(rel))) {
			return true
		}
	}
	return false
}

// profileRoots — корни, внутри которых лежат папки профилей этого браузера.
func profileRoots(def *resolved) []string {
	var out []string
	if def.spec.Family == FamilyFirefox {
		if def.roamingRoot != "" {
			out = append(out, def.roamingRoot)
		}
		if def.localRoot != "" {
			out = append(out, def.localRoot)
		}
	} else if def.userData != "" {
		out = append(out, def.userData)
	}
	return out
}

// freeRelPath подбирает свободный путь для нового профиля.
func freeRelPath(def *resolved, current []Profile) string {
	stamp := timestamp()
	if def.spec.Family == FamilyFirefox {
		// Держим ту же раскладку, что у существующих профилей: если они лежат
		// в подпапке «Profiles/», новый тоже кладём туда.
		prefix := ""
		for _, p := range current {
			if d := filepath.ToSlash(filepath.Dir(filepath.FromSlash(p.RelPath))); d != "." && d != "" {
				prefix = d + "/"
				break
			}
		}
		base := prefix + "restored-" + stamp
		rel := base
		for i := 2; relPathTaken(def, current, rel); i++ {
			rel = base + "-" + strconv.Itoa(i)
		}
		return rel
	}
	// Chromium: свободный «Profile N».
	for i := 2; ; i++ {
		name := "Profile " + strconv.Itoa(i)
		if !relPathTaken(def, current, name) {
			return name
		}
	}
}

// uniqueSiblingPath добавляет -2, -3…, если путь уже существует.
func uniqueSiblingPath(p string) string {
	if !pathExists(p) {
		return p
	}
	for i := 2; ; i++ {
		cand := p + "-" + strconv.Itoa(i)
		if !pathExists(cand) {
			return cand
		}
	}
}

// registerNewProfile прописывает созданный профиль в конфигурации браузера,
// чтобы он появился в списке профилей (иначе данные лежат на диске, но
// браузер о них не знает).
func registerNewProfile(def *resolved, relPath, name string) error {
	if def.spec.Family == FamilyFirefox {
		return registerFirefoxProfile(def.roamingRoot, relPath, name)
	}
	return registerChromiumProfile(def.userData, relPath, name)
}

// registerChromiumProfile добавляет запись в profile.info_cache файла
// «Local State». Только этот файл и правим: остальные его поля (ключи
// шифрования и настройки установки) должны остаться нетронутыми, поэтому
// работаем с сырыми JSON-фрагментами, а не с разобранной структурой.
func registerChromiumProfile(userData, relPath, name string) error {
	if userData == "" || relPath == "" {
		return nil
	}
	path := filepath.Join(userData, "Local State")
	var root map[string]json.RawMessage
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &root)
	}
	if root == nil {
		root = map[string]json.RawMessage{}
	}
	prof := map[string]json.RawMessage{}
	if raw, ok := root["profile"]; ok {
		json.Unmarshal(raw, &prof)
	}
	cache := map[string]json.RawMessage{}
	if raw, ok := prof["info_cache"]; ok {
		json.Unmarshal(raw, &cache)
	}
	entry, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return err
	}
	cache[relPath] = entry
	if prof["info_cache"], err = json.Marshal(cache); err != nil {
		return err
	}
	if root["profile"], err = json.Marshal(prof); err != nil {
		return err
	}
	out, err := json.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// registerFirefoxProfile добавляет секцию [ProfileN] в profiles.ini.
// Остальные секции не трогаем: пользователь мог настроить их сам.
//
// Если секция с этим путём уже есть (например, профиль был, а папку удалили),
// вторую не добавляем — иначе в списке профилей появился бы дубликат, и
// браузер мог бы ругаться на повторяющийся Path.
func registerFirefoxProfile(roamingRoot, relPath, name string) error {
	if roamingRoot == "" || relPath == "" {
		return nil
	}
	ini := filepath.Join(roamingRoot, "profiles.ini")
	data, _ := os.ReadFile(ini)
	text := strings.TrimPrefix(string(data), "\uFEFF")
	want := filepath.ToSlash(relPath)
	for _, e := range parseProfilesIni(ini) {
		if sameRelPath(e.Path, want) {
			return nil // уже прописан
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	text += "[Profile" + strconv.Itoa(nextProfileIndex(text)) + "]\n" +
		"Name=" + name + "\n" +
		"IsRelative=1\n" +
		"Path=" + want + "\n"
	return os.WriteFile(ini, []byte(text), 0644)
}

// nextProfileIndex находит следующий свободный номер секции [ProfileN].
func nextProfileIndex(ini string) int {
	max := -1
	for _, line := range strings.Split(ini, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[Profile") || !strings.HasSuffix(line, "]") {
			continue
		}
		num := strings.TrimSuffix(strings.TrimPrefix(line, "[Profile"), "]")
		if n, err := strconv.Atoi(num); err == nil && n > max {
			max = n
		}
	}
	return max + 1
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
		if e.Name() == "manifest.json" || isReadmeFile(e.Name()) {
			continue
		}
		out = append(out, SourceItem{Label: e.Name(), Path: filepath.Join(dir, e.Name()), IsFile: true})
	}
	return out
}

// mapSourceToTarget сопоставляет источник бэкапа с путём на новой системе.
//
// «Root» (Local State у Chromium) намеренно НЕ восстанавливается: это общий
// для всех профилей файл установки, а не данные профиля. Копирование его из
// бэкапа затирало список профилей текущей системы — как раз одна из причин
// «наложения» профилей друг на друга.
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

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings — настройки приложения. В portable-режиме лежат РЯДОМ С .exe
// (файл BrowserBackup-settings.json), чтобы папку с программой можно было
// целиком носить на флешке. В системе ничего не создаётся: ни %APPDATA%,
// ни реестр — только выбранные пользователем бэкапы.
type Settings struct {
	BackupDir string `json:"backup_dir"`
	Language  string `json:"language"` // "en" (по умолчанию) | "ru"
	// CustomProfileDirs — папки профилей, добавленные вручную
	// (режим «Другой браузер»). Каждая запись — произвольная папка на диске.
	CustomProfileDirs []string `json:"custom_profile_dirs,omitempty"`
	// BackupDirSet — папка бэкапа уже выбрана пользователем (или programmatically
	// в старой версии). Пока false, при запуске открывается диалог выбора:
	// путь по умолчанию (D:\BrowserBackup и т.п.) подходит не всем.
	BackupDirSet bool `json:"backup_dir_set,omitempty"`
	// FirstRunDone — первичная настройка (выбор папки бэкапа) пройдена.
	// Нужна отдельно от BackupDirSet: при первом запуске путь по умолчанию
	// вычисляется и без выбора пользователя, и его нельзя считать «выбором».
	FirstRunDone bool `json:"first_run_done,omitempty"`
}

// settingsPath — файл настроек рядом с .exe (portable-режим).
func settingsPath() string {
	return filepath.Join(exeDir(), "BrowserBackup-settings.json")
}

// loadSettings читает настройки. Если файла нет — возвращает настройки с
// папкой бэкапа по умолчанию, но с FirstRunDone=false: тогда интерфейс
// попросит пользователя выбрать папку самому.
func loadSettings() Settings {
	st := Settings{Language: defaultLang}
	if data, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(data, &st)
	}
	// Значение по умолчанию, если путь не задан: предпочитаем диск, отличный
	// от C:, чтобы бэкап пережил переустановку Windows.
	if st.BackupDir == "" {
		st.BackupDir = defaultBackupDir()
	}
	if st.Language == "" {
		st.Language = defaultLang
	}
	return st
}

// needsFirstRun сообщает, что пользователю при запуске нужно выбрать папку
// для бэкапов. Старый формат настроек (без флага) с уже записанным путём
// считается настроенным — выбор папки в интерфейсе его туда и записывал;
// просить выбрать заново ради этого не стоит.
func (st Settings) needsFirstRun() bool {
	if st.FirstRunDone {
		return false
	}
	return !(st.BackupDirSet && st.BackupDir != "")
}

// defaultBackupDir выбирает папку для бэкапа по умолчанию (только как место,
// которое откроется в диалоге выбора): первый существующий диск, кроме C:
// (D:, E:, …), иначе папка пользователя.
func defaultBackupDir() string {
	for _, letter := range []string{"D", "E", "F", "G"} {
		root := letter + ":\\"
		if pathExists(root) {
			return filepath.Join(root, "BrowserBackup")
		}
	}
	return filepath.Join(os.Getenv("USERPROFILE"), "BrowserBackup")
}

func saveSettings(st Settings) {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	// Папка exe уже существует. Если она только для чтения (программа
	// запущена с CD/защищённого носителя) — просто не сохраняем; это не
	// ошибка, программа продолжит работать с текущими настройками.
	if err := os.WriteFile(settingsPath(), data, 0644); err != nil {
		log.Log("settings: не удалось сохранить %s: %v", settingsPath(), err)
	}
}

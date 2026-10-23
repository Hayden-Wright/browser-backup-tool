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
}

// settingsPath — файл настроек рядом с .exe (portable-режим).
func settingsPath() string {
	return filepath.Join(exeDir(), "BrowserBackup-settings.json")
}

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

// defaultBackupDir выбирает папку для бэкапа по умолчанию:
// первый существующий диск, кроме C: (D:, E:, …), иначе папка пользователя.
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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// exeDir — папка, где лежит .exe (portable-корень). Здесь же хранятся
// настройки (BrowserBackup-settings.json) и читается необязательный
// внешний browsers.json — чтобы папку с программой можно было носить целиком.
func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

// humanSize — размер в удобном виде: 1.2 GB, 340 MB, 12 KB.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	f := float64(n)
	i := -1
	for f >= unit && i < len(units)-1 {
		f /= unit
		i++
	}
	if f >= 100 {
		return fmt.Sprintf("%.0f %s", f, units[i])
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

// dirSize считает суммарный размер файлов в папке (без символических ссылок).
func dirSize(root string) int64 {
	var total int64
	filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// isRunning сообщает, запущен ли процесс с одним из указанных имён (без .exe).
func isRunning(names ...string) bool {
	procs := runningSet()
	for _, n := range names {
		if procs[strings.ToLower(n)] {
			return true
		}
	}
	return false
}

// runningSet возвращает множество запущенных процессов (нижний регистр, без .exe).
func runningSet() map[string]bool {
	set := map[string]bool{}
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return set
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// первая колонка CSV — имя образа в кавычках
		fields := strings.Split(line, ",")
		if len(fields) == 0 {
			continue
		}
		name := strings.Trim(fields[0], "\" \r\n")
		name = strings.TrimSuffix(strings.ToLower(name), ".exe")
		if name != "" {
			set[name] = true
		}
	}
	return set
}

// sanitize делает строку безопасной для имени папки.
func sanitize(name string) string {
	repl := strings.NewReplacer(
		"\\", "_", "/", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
	)
	name = repl.Replace(name)
	name = strings.Trim(name, " .")
	if name == "" {
		name = "profile"
	}
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}

// timestamp — метка времени для имён папок.
func timestamp() string {
	return time.Now().Format("2006-01-02_15-04-05")
}

// listDirs возвращает отсортированные подпапки каталога.
func listDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// pathExists проверяет существование пути.
func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// openInExplorer открывает папку в проводнике, а файл — приложением по
// умолчанию системы (например, .log откроется в «Блокноте»).
//
// ВАЖНО: explorer НЕ передаём в hideConsole. Он GUI-приложение и консоли не
// создаёт, а hideConsole выставляет STARTUPINFO.wShowWindow = SW_HIDE — из-за
// этого explorer создавал окно СКРЫТЫМ, и кнопки «Открыть папку» / «Открыть
// файл лога» выглядели как «ничего не происходит». Для консольных утилит
// (tasklist, cmd, powershell) hideConsole по-прежнему нужен.
func openInExplorer(path string) {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		// Папка — в проводнике (hideConsole тут нельзя, см. выше).
		exec.Command("explorer", path).Start()
		return
	}
	// Файл — приложением по умолчанию (ShellExecute), а не /select в проводнике.
	if err := openViaShell(path); err != nil {
		exec.Command("explorer", "/select,"+path).Start()
	}
}

// windowsVersion возвращает понятное имя версии Windows.
func windowsVersion() string {
	cmd := exec.Command("cmd", "/c", "ver")
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "Windows"
	}
	s := strings.TrimSpace(string(out))
	// пример: "Microsoft Windows [Version 10.0.22631.4317]"
	if i := strings.Index(s, "["); i >= 0 {
		s = strings.Trim(s[i+1:], "[] ")
	}
	s = strings.TrimPrefix(s, "Version ")
	parts := strings.Split(s, ".")
	if len(parts) >= 3 {
		build := parts[2]
		switch {
		case build >= "22000":
			return "Windows 11 (build " + build + ")"
		case build >= "10240":
			return "Windows 10 (build " + build + ")"
		}
	}
	return "Windows " + s
}

// freeSpace возвращает свободное место на диске, содержащем путь (в байтах).
func freeSpace(path string) int64 {
	vol := filepath.VolumeName(path)
	if vol == "" {
		vol = path
	}
	// Используем PowerShell: надёжно и без внешних зависимостей.
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf("(Get-PSDrive -Name '%s' -ErrorAction SilentlyContinue).Free", strings.TrimSuffix(vol, ":")))
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	var n int64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n)
	if n <= 0 {
		return -1
	}
	return n
}

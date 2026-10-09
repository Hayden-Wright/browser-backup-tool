package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Logger пишет журнал в файл и (опционально) отдаёт строки в интерфейс.
type Logger struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	onLine  func(string)
	enabled bool
}

var log = &Logger{}

// SetSink задаёт функцию, вызываемую для каждой новой строки (для окна журнала).
func (l *Logger) SetSink(fn func(string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onLine = fn
}

// SetFile переключает файл журнала (например, при смене папки бэкапа).
func (l *Logger) SetFile(dir string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	if dir == "" {
		return
	}
	os.MkdirAll(dir, 0755)
	p := filepath.Join(dir, "browser-backup.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	l.file = f
	l.path = p
	l.enabled = true
}

// Path возвращает путь к текущему файлу журнала.
func (l *Logger) Path() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// Log записывает строку в журнал.
func (l *Logger) Log(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s  %s", time.Now().Format("2006-01-02 15:04:05"), msg)

	l.mu.Lock()
	if l.file != nil {
		l.file.WriteString(line + "\r\n")
		l.file.Sync()
	}
	sink := l.onLine
	l.mu.Unlock()

	if sink != nil {
		sink(line)
	}
}

// LogPlain пишет строку без отметки времени (для многострочных блоков).
func (l *Logger) LogPlain(text string) {
	l.mu.Lock()
	if l.file != nil {
		l.file.WriteString(text + "\r\n")
		l.file.Sync()
	}
	sink := l.onLine
	l.mu.Unlock()
	if sink != nil {
		sink(text)
	}
}

// FriendlyError превращает техническую ошибку в понятное сообщение.
func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "not enough space"), strings.Contains(low, "disk full"):
		return T("err_no_space")
	case strings.Contains(low, "access is denied"), strings.Contains(low, "permission denied"),
		strings.Contains(low, "отказано в доступе"):
		return T("err_access")
	}
	return Tf("err_generic", msg)
}

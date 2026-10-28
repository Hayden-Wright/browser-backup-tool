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
//
// Файл журнала — ВРЕМЕННЫЙ. Программа не оставляет следов рядом с бэкапами:
// журнал создаётся во временной папке при старте (OpenTemp) и удаляется при
// выходе (Close). Пользователю журнал виден в окне программы; по умолчанию
// файл вообще не ведётся — так ведут себя тесты и скрытый CLI-режим.
type Logger struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	tempDir string // каталог, который удаляем при закрытии
	onLine  func(string)
}

var log = &Logger{}

// SetSink задаёт функцию, вызываемую для каждой новой строки (для окна журнала).
func (l *Logger) SetSink(fn func(string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onLine = fn
}

// OpenTemp открывает журнал во временной папке Windows: создаёт каталог
// «BrowserBackup-log-*» и файл browser-backup.log внутри. Каталог удаляется
// при выходе через Close. Если открыть файл не удалось — журнал продолжит
// работать только в окне.
func (l *Logger) OpenTemp() {
	dir, err := os.MkdirTemp("", "BrowserBackup-log-")
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "browser-backup.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		os.RemoveAll(dir)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
	}
	l.file = f
	l.path = f.Name()
	l.tempDir = dir
}

// Close закрывает файл журнала и удаляет временный каталог. Файл закрываем
// ДО удаления: на Windows открытый файл удалить нельзя.
func (l *Logger) Close() {
	l.mu.Lock()
	if l.file == nil {
		l.mu.Unlock()
		return
	}
	l.file.Close()
	dir := l.tempDir
	l.file = nil
	l.path = ""
	l.tempDir = ""
	l.mu.Unlock()

	if dir != "" {
		os.RemoveAll(dir)
	}
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

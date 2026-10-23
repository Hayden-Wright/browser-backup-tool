//go:build !windows

package main

// runningExePaths — заглушка для не-Windows сборок: пути процессов
// недоступны, поэтому детект браузеров работает только по именам.
// Реальная реализация — proc_windows.go.
func runningExePaths() []string { return nil }

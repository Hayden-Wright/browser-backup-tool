//go:build windows

// Browser Profile Backup — портативная утилита для сохранения и восстановления
// профилей браузеров. Работает локально и не оставляет следов в системе,
// кроме самих бэкапов (и, при желании, файла настроек рядом с .exe).
package main

import "os"

func main() {
	// Скрытый CLI-режим: browser-backup.exe -cli -action ...
	for _, a := range os.Args[1:] {
		if a == "-cli" || a == "--cli" {
			os.Exit(runCLI(os.Args[2:]))
		}
	}
	runApp()
}

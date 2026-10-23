//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// runningExePaths возвращает полные пути запущенных процессов в нижнем
// регистре, с прямыми слэшами: "c:/program files/imput/helium/application/chrome.exe".
//
// Зачем это нужно. Helium — форк Chromium, и его исполняемый файл называется
// chrome.exe (см. imputnet/helium-windows#29). Обычная проверка по имени
// процесса не отличит его от Google Chrome: при запущенном Chrome программа
// считала бы запущенным и Helium. Поэтому для браузеров, у которых задан
// ProcPath, решение принимается по пути к .exe, а не по имени.
//
// Ошибки не считаются фатальными: если путь процесса получить нельзя
// (например, процесс запущен от другого пользователя), он просто не попадёт
// в список, и вызывающий код откатится на проверку по имени.
func runningExePaths() []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	var out []string
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if p := processImagePath(entry.ProcessID); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// processImagePath возвращает нормализованный путь к исполняемому файлу
// процесса или "" если его не удалось получить.
func processImagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return normalizePath(windows.UTF16ToString(buf[:size]))
}

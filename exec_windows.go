//go:build windows

package main

import (
	"os/exec"
	"syscall"
	"unsafe"
)

// openViaShell открывает файл приложением, назначенным в системе по умолчанию
// (ShellExecuteW). Для .log это, как правило, «Блокнот». Возвращает ошибку,
// если ассоциация не сработала (тогда вызывающий откатывается на проводник).
func openViaShell(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("open")
	r, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(p)), 0, 0, swShowNormal)
	if r <= 32 { // ShellExecute возвращает >32 при успехе
		return syscall.Errno(r)
	}
	return nil
}

var procShellExecuteW = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

const swShowNormal = 1

// hideConsole запрещает создание отдельного окна консоли для дочернего
// процесса (tasklist, cmd, powershell). Без этого при каждом вызове на экране
// на секунду мелькает чёрное окно cmd.
//
// НЕ применять к explorer: он GUI-приложение, а флаг выставляет
// STARTUPINFO.wShowWindow=SW_HIDE — окно проводника откроется скрытым
// (см. openInExplorer в util.go).
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}

// applyDarkTitleBar делает заголовок окна тёмным (Windows 10 1809+), чтобы
// системная полоса свернуть/закрыть не выбивалась из тёмно-синей темы.
func applyDarkTitleBar(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	proc := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	val := int32(1)
	// 20 — DWMWA_USE_IMMERSIVE_DARK_MODE (Win10 1903+), 19 — ранние сборки.
	for _, attr := range []uintptr{20, 19} {
		r, _, _ := proc.Call(hwnd, attr, uintptr(unsafe.Pointer(&val)), unsafe.Sizeof(val))
		if r == 0 {
			return
		}
	}
}

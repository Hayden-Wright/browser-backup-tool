//go:build !windows

package main

import "os/exec"

// hideConsole — заглушка для не-Windows сборок (реальная версия в exec_windows.go).
func hideConsole(cmd *exec.Cmd) {}

// openViaShell — заглушка для не-Windows сборок.
func openViaShell(path string) error { return errNoShellOpen }

var errNoShellOpen = errStub("openViaShell: not supported")

type errStub string

func (e errStub) Error() string { return string(e) }

// applyDarkTitleBar — заглушка для не-Windows сборок.
func applyDarkTitleBar(hwnd uintptr) {}

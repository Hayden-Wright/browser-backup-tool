//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// runCLI — скрытый режим командной строки (для проверки/автоматизации).
// Запуск:  browser-backup.exe -cli -action scan|backup|restore -dir <папка> [-browsers zen,chrome]
func runCLI(args []string) int {
	fs := flag.NewFlagSet("cli", flag.ContinueOnError)
	action := fs.String("action", "scan", "scan | backup | restore")
	dir := fs.String("dir", "", "папка бэкапа")
	browsers := fs.String("browsers", "", "список браузеров через запятую")
	_ = fs.Parse(args)

	// Лог в stdout для CLI.
	log.SetSink(func(line string) { fmt.Println(line) })
	if *dir != "" {
		os.MkdirAll(*dir, 0755)
		log.SetFile(*dir)
	}

	keys := map[string]bool{}
	if *browsers != "" {
		for _, k := range strings.Split(*browsers, ",") {
			keys[strings.TrimSpace(k)] = true
		}
	}

	switch *action {
	case "scan":
		for _, p := range allProfiles() {
			mark := ""
			if p.IsDefault {
				mark = " [default]"
			}
			fmt.Printf("%s\t%s%s\t%s\n", p.BrowserKey, p.Name, mark, p.RelPath)
		}
		return 0

	case "backup":
		var sel []Profile
		for _, p := range allProfiles() {
			if len(keys) == 0 || keys[p.BrowserKey] {
				sel = append(sel, p)
			}
		}
		if len(sel) == 0 {
			fmt.Println("нет профилей для бэкапа")
			return 1
		}
		res, err := doBackup(sel, *dir, &atomic.Bool{}, nil)
		if err != nil {
			fmt.Println("ОШИБКА:", friendlyError(err))
			return 1
		}
		fmt.Printf("Итог: профилей=%d, размер=%s, пропущено=%d\n", res.Count, humanSize(res.Bytes), res.Skipped)
		return 0

	case "restore":
		all := listBackups(*dir)
		var sel []manifest
		for _, m := range all {
			if len(keys) == 0 || keys[m.BrowserKey] {
				sel = append(sel, m)
			}
		}
		if len(sel) == 0 {
			fmt.Println("нет бэкапов для восстановления")
			return 1
		}
		res, err := doRestore(sel, *dir, &atomic.Bool{}, nil)
		if err != nil {
			fmt.Println("ОШИБКА:", friendlyError(err))
			return 1
		}
		fmt.Printf("Итог: восстановлено=%d, размер=%s, пропущено=%d\n", res.Count, humanSize(res.Bytes), res.Skipped)
		if len(res.Broken) > 0 {
			fmt.Println("ПРОПУЩЕНЫ битые бэкапы:", strings.Join(res.Broken, ", "))
		}
		if res.SafetyDir != "" {
			fmt.Println("Страховочная копия:", res.SafetyDir)
		}
		if len(res.NotRun) > 0 {
			fmt.Println("Сначала запустите один раз:", strings.Join(res.NotRun, ", "))
		}
		return 0
	}

	fmt.Println("неизвестное действие:", *action)
	return 2
}

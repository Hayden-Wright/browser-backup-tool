//go:build windows

package main

import (
	"encoding/json"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// Диалог выбора папки реализован здесь напрямую через COM (IFileOpenDialog),
// без внешней библиотеки.
//
// Почему не go-common-file-dialog: её v1.2.0 вызывает COM-методы через
// syscall.Syscall(fn, 1, this, arg, 0) — с числом аргументов 1, хотя
// аргументов два. В старых версиях Go этот счётчик на amd64 игнорировался и
// второй аргумент всё равно попадал в RDX. Начиная с Go 1.17+ рантайм
// (syscall_syscalln → asmstdcall) стал соблюдать счётчик: при n=1 на стек
// кладётся только `this`, а параметр теряется. COM-метод читает мусор в RDX
// и процесс падает с нарушением доступа 0xc0000005 — именно поэтому кнопки
// «Изменить…» и «Добавить папку…» закрывали программу.
//
// Здесь все вызовы идут через comCall (syscall.SyscallN), который берёт число
// аргументов из самого вариативного вызова — ошибиться в нём нельзя.

// ---------- COM-интерфейсы (раскладка vtable) ----------

type iUnknownVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

// iModalWindowVtbl — IModalWindow (после IUnknown идёт Show).
type iModalWindowVtbl struct {
	iUnknownVtbl
	Show uintptr
}

// iFileDialogVtbl — IFileDialog.
type iFileDialogVtbl struct {
	iModalWindowVtbl
	SetFileTypes        uintptr
	SetFileTypeIndex    uintptr
	GetFileTypeIndex    uintptr
	Advise              uintptr
	Unadvise            uintptr
	SetOptions          uintptr
	GetOptions          uintptr
	SetDefaultFolder    uintptr
	SetFolder           uintptr
	GetFolder           uintptr
	GetCurrentSelection uintptr
	SetFileName         uintptr
	GetFileName         uintptr
	SetTitle            uintptr
	SetOkButtonLabel    uintptr
	SetFileNameLabel    uintptr
	GetResult           uintptr
	AddPlace            uintptr
	SetDefaultExtension uintptr
	Close               uintptr
	SetClientGuid       uintptr
	ClearClientData     uintptr
	SetFilter           uintptr
}

// iFileOpenDialogVtbl — IFileOpenDialog.
type iFileOpenDialogVtbl struct {
	iFileDialogVtbl
	GetResults       uintptr
	GetSelectedItems uintptr
}

type iShellItemVtbl struct {
	iUnknownVtbl
	BindToHandler  uintptr
	GetParent      uintptr
	GetDisplayName uintptr
	GetAttributes  uintptr
	Compare        uintptr
}

var (
	procSHCreateItemFromParsingName = windows.NewLazySystemDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")

	clsidFileOpenDialog = ole.NewGUID("{DC1C5A9C-E88A-4dde-A5A1-60F82A20AEF7}")
	iidFileOpenDialog   = ole.NewGUID("{d57c7288-d4ad-4768-be02-9d969532d960}")
	iidIShellItem       = ole.NewGUID("43826d1e-e718-42ee-bc55-a1e261c37bfe")
)

// Значения FILEOPENDIALOGOPTIONS.
const (
	fosPickFolders     = 0x00000020
	fosForceFileSystem = 0x00000040
	fosPathMustExist   = 0x00000800
)

// SIGDN_FILESYSPATH — получить файловый путь вместо отображаемого имени.
const sigdnFileSysPath = 0x80058000

// ---------- низкоуровневые обёртки ----------

// comCall вызывает COM-метод по адресу слота vtable. Число аргументов
// определяется самим вызовом, поэтому «this» и параметры передаются всегда.
func comCall(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(fn, args...)
	return r
}

// hresultErr превращает HRESULT в error (nil, если код успешный).
func hresultErr(hr uintptr) error {
	if int32(hr) < 0 {
		return ole.NewError(hr)
	}
	return nil
}

// comRelease уменьшает счётчик ссылок COM-объекта.
func comRelease(obj uintptr) {
	if obj == 0 {
		return
	}
	vtbl := *(**iUnknownVtbl)(unsafe.Pointer(obj))
	comCall(vtbl.Release, obj)
}

// newShellItem создаёт IShellItem по файловому пути.
func newShellItem(path string) (uintptr, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var item uintptr
	hr, _, _ := procSHCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(p)),
		0,
		uintptr(unsafe.Pointer(iidIShellItem)),
		uintptr(unsafe.Pointer(&item)),
	)
	runtime.KeepAlive(p)
	if err := hresultErr(hr); err != nil {
		return 0, err
	}
	return item, nil
}

// shellItemPath возвращает файловый путь IShellItem.
func shellItemPath(item uintptr) (string, error) {
	vtbl := *(**iShellItemVtbl)(unsafe.Pointer(item))
	var name *uint16
	hr := comCall(vtbl.GetDisplayName, item, sigdnFileSysPath,
		uintptr(unsafe.Pointer(&name)))
	if err := hresultErr(hr); err != nil {
		return "", err
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(name)))
	return ole.LpOleStrToString(name), nil
}

// newFolderDialog создаёт COM-объект IFileOpenDialog.
func newFolderDialog() (uintptr, error) {
	unk, err := ole.CreateInstance(clsidFileOpenDialog, iidFileOpenDialog)
	if err != nil {
		return 0, err
	}
	return uintptr(unsafe.Pointer(unk)), nil
}

// setDialogTitle задаёт заголовок окна диалога.
func setDialogTitle(dlg uintptr, title string) {
	if title == "" {
		return
	}
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	vtbl := *(**iFileOpenDialogVtbl)(unsafe.Pointer(dlg))
	comCall(vtbl.SetTitle, dlg, uintptr(unsafe.Pointer(t)))
	runtime.KeepAlive(t)
}

// setDialogFolder задаёт папку, которая откроется при показе диалога.
// Пробуем SetFolder (жёстко), при неудаче — SetDefaultFolder (мягко).
func setDialogFolder(dlg uintptr, folder string) {
	if folder == "" {
		return
	}
	item, err := newShellItem(folder)
	if err != nil {
		return // папки может не быть — диалог откроется в последнем месте
	}
	defer comRelease(item)

	vtbl := *(**iFileOpenDialogVtbl)(unsafe.Pointer(dlg))
	if hr := comCall(vtbl.SetFolder, dlg, item); int32(hr) < 0 {
		comCall(vtbl.SetDefaultFolder, dlg, item)
	}
}

// setPickFolders включает режим выбора папок (а не файлов).
func setPickFolders(dlg uintptr) error {
	vtbl := *(**iFileOpenDialogVtbl)(unsafe.Pointer(dlg))
	var opts uint32
	if hr := comCall(vtbl.GetOptions, dlg, uintptr(unsafe.Pointer(&opts))); int32(hr) < 0 {
		return ole.NewError(hr)
	}
	opts |= fosPickFolders | fosForceFileSystem | fosPathMustExist
	if hr := comCall(vtbl.SetOptions, dlg, uintptr(opts)); int32(hr) < 0 {
		return ole.NewError(hr)
	}
	return nil
}

// showModal показывает диалог; parent — HWND окна-владельца (модальность).
func showModal(dlg, parent uintptr) error {
	vtbl := *(**iFileOpenDialogVtbl)(unsafe.Pointer(dlg))
	hr := comCall(vtbl.Show, dlg, parent)
	if int32(hr) < 0 {
		return ole.NewError(hr)
	}
	return nil
}

// dialogResultPath забирает выбранный путь (E_FAIL/отмена → пусто).
func dialogResultPath(dlg uintptr) (string, error) {
	vtbl := *(**iFileOpenDialogVtbl)(unsafe.Pointer(dlg))
	var item uintptr
	hr := comCall(vtbl.GetResult, dlg, uintptr(unsafe.Pointer(&item)))
	if int32(hr) < 0 {
		return "", ole.NewError(hr)
	}
	if item == 0 {
		return "", errDialogCancelled
	}
	defer comRelease(item)
	return shellItemPath(item)
}

// errDialogCancelled — пользователь закрыл диалог, это не ошибка.
var errDialogCancelled = ole.NewError(0x800704C7) // HRESULT_FROM_WIN32(ERROR_CANCELLED)

// showFolderDialog выполняет показ диалога. Вызывается ТОЛЬКО в потоке окна
// (он STA с циклом сообщений), поэтому модальный цикл диалога с ним уживается.
func showFolderDialog(title, startDir string, parent uintptr) folderPick {
	dlg, err := newFolderDialog()
	if err != nil {
		return folderPick{Err: friendlyError(err)}
	}
	defer comRelease(dlg)

	setDialogTitle(dlg, title)
	if err := setPickFolders(dlg); err != nil {
		return folderPick{Err: friendlyError(err)}
	}
	setDialogFolder(dlg, startDir)

	if err := showModal(dlg, parent); err != nil {
		if isCancelled(err) {
			return folderPick{} // закрытие/отмена — не ошибка
		}
		return folderPick{Err: friendlyError(err)}
	}

	path, err := dialogResultPath(dlg)
	if err != nil {
		if isCancelled(err) {
			return folderPick{}
		}
		return folderPick{Err: friendlyError(err)}
	}
	if path == "" {
		return folderPick{}
	}
	return folderPick{Path: path, OK: true}
}

// isCancelled распознаёт отмену пользователем (HRESULT 0x800704C7).
func isCancelled(err error) bool {
	if oleErr, ok := err.(*ole.OleError); ok {
		return uint32(oleErr.Code()) == 0x800704C7
	}
	return false
}

// ---------- мост в JavaScript ----------

// folderPick — результат работы диалога выбора папки.
type folderPick struct {
	Path string `json:"path"`
	OK   bool   `json:"ok"` // false — пользователь закрыл диалог
	Err  string `json:"error,omitempty"`
}

var (
	pickBusy atomic.Bool
	pickMu   sync.Mutex
	pickRes  *folderPick
)

// jsPickFolder ставит показ диалога в очередь потока окна и сразу отвечает —
// ждать не нужно. Результат JS забирает опросом window.api.pollFolder().
func (a *App) jsPickFolder(title, startDir string) (string, error) {
	if a.w == nil {
		return errJSON("error", "no window"), nil
	}
	if !pickBusy.CompareAndSwap(false, true) {
		return "{}", nil // диалог уже открыт
	}
	pickMu.Lock()
	pickRes = nil
	pickMu.Unlock()

	// Значения захватываем: замыкание выполнится позже, в потоке окна.
	t, start, parent := title, startDir, a.hwnd()
	a.w.Dispatch(func() {
		res := showFolderDialog(t, start, parent)
		pickMu.Lock()
		pickRes = &res
		pickMu.Unlock()
	})
	return "{}", nil
}

// jsPollFolder возвращает результат диалога, если он уже закрылся.
func (a *App) jsPollFolder() (string, error) {
	pickMu.Lock()
	r := pickRes
	pickMu.Unlock()
	if r == nil {
		return "{}", nil // ещё открыт
	}
	pickBusy.Store(false)
	data, _ := json.Marshal(*r)
	return string(data), nil
}

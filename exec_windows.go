//go:build windows

package main

import (
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
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

// ---------- Своя шапка окна (frameless) ----------

// Константы Win32, нужные для безрамочного окна. Объявлены здесь, чтобы не
// тянуть ещё одну обёртку над user32 ради нескольких чисел; значения из
// winuser.h.
const (
	gwlStyle        = ^uintptr(15) // -16, GWL_STYLE
	gwlpWndProc     = ^uintptr(3)  // -4,  GWLP_WNDPROC
	wsCaption       = 0x00C00000
	wsSysMenu       = 0x00080000
	wsMinimizeBox   = 0x00020000
	wsMaximizeBox   = 0x00010000
	wsThickFrame    = 0x00040000
	wsBorder        = 0x00800000
	swpFrameChanged = 0x0020
	swpNoZOrder     = 0x0004
	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	wmNCHitTest     = 0x0084
	wmNCLButtonDown = 0x00A1
	htCaption       = 2
	htClient        = 1
	swMinimize      = 6
	swMaximize      = 3
	swRestore       = 9
	wmSysCommand    = 0x0112
	scMinimize      = 0xF020
	scMaximize      = 0xF030
	scRestore       = 0xF120
	wmGetMinMaxInfo = 0x0024
	wmNCCalcSize    = 0x0083
	htLeft          = 10
	htRight         = 11
	htTop           = 12
	htTopLeft       = 13
	htTopRight      = 14
	htBottom        = 15
	htBottomLeft    = 16
	htBottomRight   = 17
)

// Метрики рамки и флаги монитора — для прямоугольника разворота.
const (
	monitorDefaultToNearest = 2
	smCxSizeFrame           = 32
	smCySizeFrame           = 33
	smCxPaddedBorder        = 92
)

var (
	procSetWindowLongPtr  = syscall.NewLazyDLL("user32.dll").NewProc("SetWindowLongPtrW")
	procGetWindowLongPtr  = syscall.NewLazyDLL("user32.dll").NewProc("GetWindowLongPtrW")
	procSetWindowPos      = syscall.NewLazyDLL("user32.dll").NewProc("SetWindowPos")
	procCallWindowProc    = syscall.NewLazyDLL("user32.dll").NewProc("CallWindowProcW")
	procScreenToClient    = syscall.NewLazyDLL("user32.dll").NewProc("ScreenToClient")
	procGetClientRect     = syscall.NewLazyDLL("user32.dll").NewProc("GetClientRect")
	procIsZoomed          = syscall.NewLazyDLL("user32.dll").NewProc("IsZoomed")
	procPostMessage       = syscall.NewLazyDLL("user32.dll").NewProc("PostMessageW")
	procMonitorFromWindow = syscall.NewLazyDLL("user32.dll").NewProc("MonitorFromWindow")
	procGetMonitorInfo    = syscall.NewLazyDLL("user32.dll").NewProc("GetMonitorInfoW")
	procGetSystemMetrics  = syscall.NewLazyDLL("user32.dll").NewProc("GetSystemMetrics")
	procGetCursorPos      = syscall.NewLazyDLL("user32.dll").NewProc("GetCursorPos")
	procReleaseCapture    = syscall.NewLazyDLL("user32.dll").NewProc("ReleaseCapture")
)

// dragState — область перетаскивания окна, в координатах клиента.
// Обновляется из JS: высота шапки и ширина полосы кнопок справа, которую
// перетаскивание не должно перехватывать.
var dragState struct {
	height       int32
	excludeRight int32
}

type rect struct{ left, top, right, bottom int32 }
type point struct{ x, y int32 }

// minMaxInfo — структура из WM_GETMINMAXINFO: окно может здесь поправить свои
// пределы, в том числе прямоугольник разворота. Раскладка из winuser.h,
// все поля — LONG.
type minMaxInfo struct {
	reserved     point
	maxSize      point
	maxPosition  point
	minTrackSize point
	maxTrackSize point
}

// monitorInfo — MONITORINFO из winuser.h.
type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

// prevWndProc — оконная процедура библиотеки. Через неё проходит всё, что мы
// не перехватили, поэтому поведение WebView2 не меняется.
var prevWndProc uintptr

// mainHwnd — главное окно. Нужно startDrag: WM_NCLBUTTONDOWN отправляется
// именно ему, а не окну под курсором.
var mainHwnd uintptr

// framelessWndProc — подкласс окна. Перехватывает только WM_NCHITTEST:
// без него безрамочное окно нельзя ни перетащить, ни развернуть двойным
// щелчком по шапке.
//
// Почему именно WM_NCHITTEST. Windows спрашивает у окна «что под курсором?»
// и по ответу решает, кто обработает нажатие: если ответить HTCAPTION,
// система сама начнёт перетаскивание и обработает двойной щелчок как
// «развернуть». Ничего не двигаем руками — только отвечаем на вопрос.
func framelessWndProc(hwnd, msg, wp, lp uintptr) uintptr {
	// Клиентская область занимает всё окно.
	//
	// Зачем. У окна с WS_THICKFRAME, но без WS_CAPTION, Windows всё равно
	// создаёт неприкладную область — замерено, по 8 px с каждой стороны.
	// Боковые полосы DWM рисует прозрачными, а верхнюю закрашивает белым:
	// на тёмной теме это белая полоса над шапкой. Клиентской области там не
	// нужно ничего, поэтому сообщаем, что клиент — всё окно, и полосы исчезают.
	// Рамку изменения размера возвращаем сами в WM_NCHITTEST.
	if msg == wmNCCalcSize && wp != 0 {
		return 0
	}
	// Прямоугольник разворота правим до того, как сообщение уйдёт дальше.
	// Без этого развёрнутое окно накрывает панель задач — см. fixMaximizeRect.
	if msg == wmGetMinMaxInfo && lp != 0 {
		fixMaximizeRect(hwnd, (*minMaxInfo)(unsafe.Pointer(lp)))
	}
	// Фон заливаем сами: класс окна в библиотеке зарегистрирован без кисти,
	// поэтому DefWindowProc красит окно белым — на тёмной теме это заметная
	// вспышка, пока грузится HTML.
	if msg == wmEraseBkgnd {
		fillWindowBackground(hwnd, wp)
		return 1
	}
	if msg == wmNCHitTest {
		// lp — экранные координаты, переводим в клиентские.
		var p point
		p.x = int32(int16(lp & 0xFFFF))
		p.y = int32(int16((lp >> 16) & 0xFFFF))
		procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&p)))

		var cr rect
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))

		// Рамка изменения размера. Раз неприкладной области больше нет, её
		// обязан вернуть WM_NCHITTEST — иначе окно нельзя растянуть мышью.
		if z, _, _ := procIsZoomed.Call(hwnd); z == 0 {
			if ht := resizeHitTest(p, cr, resizeBorderWidth()); ht != 0 {
				return uintptr(ht)
			}
		}

		inHeader := p.y >= 0 && p.y < dragState.height
		// Справа — кнопки «свернуть/развернуть/закрыть»: там перетаскивание
		// перехватило бы клики, поэтому отдаём область приложению.
		inButtons := dragState.excludeRight > 0 && p.x >= cr.right-dragState.excludeRight
		if inHeader && !inButtons {
			return htCaption
		}
	}
	return callPrevWndProc(hwnd, msg, wp, lp)
}

// resizeBorderWidth — толщина полосы, за которую окно тянется мышью.
// Та же, что Windows даёт окну с рамкой: SM_CXSIZEFRAME + SM_CXPADDEDBORDER
// (замерено: 4 + 4 = 8 px).
func resizeBorderWidth() int32 {
	return int32(systemMetric(smCxSizeFrame) + systemMetric(smCxPaddedBorder))
}

// resizeHitTest отвечает, за какую грань окна взят курсор, или 0 — если
// курсор не на рамке.
func resizeHitTest(p point, cr rect, b int32) int {
	left := p.x < b
	right := p.x >= cr.right-b
	top := p.y < b
	bottom := p.y >= cr.bottom-b
	switch {
	case top && left:
		return htTopLeft
	case top && right:
		return htTopRight
	case bottom && left:
		return htBottomLeft
	case bottom && right:
		return htBottomRight
	case left:
		return htLeft
	case right:
		return htRight
	case top:
		return htTop
	case bottom:
		return htBottom
	}
	return 0
}

// fixMaximizeRect задаёт прямоугольник разворота по рабочей области монитора.
//
// Зачем. У окна с заголовком Windows разворачивает по рабочей области и не
// закрывает панель задач. У окна без WS_CAPTION она считает иначе и берёт
// весь монитор: замерено, окно получало 1920x1080 при рабочей области
// 1920x1050, то есть панель задач оказывалась под окном.
//
// Как. При развороте система присылает WM_GETMINMAXINFO и ждёт ответа «куда
// разворачивать»; отвечаем рабочей областью монитора. Рамку изменения размера
// компенсируем: у развёрнутого окна WS_THICKFRAME выходит за клиентскую
// область, и без поправки окно было бы уже рабочей области на её толщину
// с каждой стороны. Толщина на Windows 10 — SM_CXSIZEFRAME +
// SM_CXPADDEDBORDER (замерено: 4 + 4 = 8 px).
func fixMaximizeRect(hwnd uintptr, mmi *minMaxInfo) {
	mon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if mon == 0 {
		return
	}
	mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return
	}
	// Клиентская область совпадает с окном (WM_NCCALCSIZE), поэтому поправка
	// на рамку не нужна: разворачиваем ровно в рабочую область.
	mmi.maxPosition.x = mi.rcWork.left
	mmi.maxPosition.y = mi.rcWork.top
	mmi.maxSize.x = mi.rcWork.right - mi.rcWork.left
	mmi.maxSize.y = mi.rcWork.bottom - mi.rcWork.top
}

// systemMetric читает системную метрику (SM_*).
func systemMetric(index int) int {
	r, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int(r)
}

func callPrevWndProc(hwnd, msg, wp, lp uintptr) uintptr {
	r, _, _ := procCallWindowProc.Call(prevWndProc, hwnd, msg, wp, lp)
	return r
}

// makeFrameless убирает системный заголовок и рамку, оставляя окно
// изменяемым по размеру, и ставит свой подкласс окна.
//
// Зачем. Стандартная шапка Windows не подходит к тёмно-синему интерфейсу:
// на Windows 10 атрибут DWMWA_USE_IMMERSIVE_DARK_MODE игнорируется (это
// видно на скриншотах — полоса светлая), а на Windows 11 выглядит чужеродно.
// Своя шапка рисуется внутри HTML и одинакова везде.
//
// Что важно не потерять вместе с рамкой: WS_THICKFRAME оставляем — без него
// окно нельзя растянуть мышью; перетаскивание и Aero Snap возвращает
// WM_NCHITTEST в framelessWndProc.
func makeFrameless(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	// ВАЖНО: это второй проход, после NewWithOptions. Библиотека вызывает
	// SetSize (см. webview.SetSize), а тот возвращает WS_MAXIMIZEBOX — вместе
	// с ним возвращается и системный заголовок. Поэтому стиль правим ещё раз,
	// уже после инициализации; хук в installCreateHook убирает только вспышку.
	style, _, _ := procGetWindowLongPtr.Call(hwnd, gwlStyle)
	style &^= wsCaption | wsSysMenu | wsMinimizeBox | wsMaximizeBox
	style |= wsThickFrame | wsBorder
	procSetWindowLongPtr.Call(hwnd, gwlStyle, style)
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
		swpFrameChanged|swpNoZOrder|swpNoMove|swpNoSize)

	mainHwnd = hwnd
	installSubclass(hwnd)
}

// installSubclass ставит свою оконную процедуру поверх библиотечной.
// Повторный вызов ничего не делает: иначе получилась бы рекурсия.
func installSubclass(hwnd uintptr) {
	if prevWndProc != 0 || hwnd == 0 {
		return
	}
	wndProcCb := syscall.NewCallback(framelessWndProc)
	prev, _, _ := procSetWindowLongPtr.Call(hwnd, gwlpWndProc, wndProcCb)
	prevWndProc = prev
}

// setDragArea задаёт область перетаскивания. Вызывается из JS при загрузке
// и при изменении размера окна: высота шапки и ширина полосы кнопок зависят
// от масштаба экрана.
func setDragArea(height, excludeRight int) {
	dragState.height = int32(height)
	dragState.excludeRight = int32(excludeRight)
}

// startDrag начинает перетаскивание окна за шапку.
//
// Почему не через WM_NCHITTEST. Дочерние окна WebView2 лежат поверх всей
// клиентской области, включая шапку, и живут на СВОИХ потоках (замерено:
// tid 640 и 12980 при главном 12600). Windows спрашивает WM_NCHITTEST у окна
// под курсором — то есть у дочернего, — и главное окно вопроса не видит:
// замерено 0 сообщений WM_NCHITTEST при движении мыши по шапке, окно не
// таскалось. Подменить процедуру чужому окну нельзя: SetWindowLongPtr
// возвращает ERROR_ACCESS_DENIED (замерено).
//
// Как. JS сообщает о нажатии на шапку, а мы просим Windows начать штатное
// перетаскивание: WM_NCLBUTTONDOWN с HTCAPTION в очередь главного окна.
// Дальше всё делает система — перетаскивание и Aero Snap.
func startDrag() {
	if mainHwnd == 0 {
		return
	}
	var p point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	procReleaseCapture.Call()
	lp := uintptr(uint32(p.x)&0xFFFF) | uintptr(uint32(p.y)&0xFFFF)<<16
	procPostMessage.Call(mainHwnd, wmNCLButtonDown, htCaption, lp)
}

// windowAction выполняет действие над окном: свернуть, развернуть/восстановить,
// закрыть. Вызывается из JS своей шапки.
func windowAction(hwnd uintptr, action string) {
	if hwnd == 0 {
		return
	}
	switch action {
	case "minimize":
		procPostMessage.Call(hwnd, wmSysCommand, scMinimize, 0)
	case "maximize":
		if z, _, _ := procIsZoomed.Call(hwnd); z != 0 {
			procPostMessage.Call(hwnd, wmSysCommand, scRestore, 0)
		} else {
			procPostMessage.Call(hwnd, wmSysCommand, scMaximize, 0)
		}
	case "close":
		procPostMessage.Call(hwnd, 0x0010, 0, 0) // WM_CLOSE
	}
}

// ---------- Показ окна только когда готово ----------

// Библиотека go-webview2 создаёт окно со стилем WS_OVERLAPPEDWINDOW и сразу
// вызывает ShowWindow — то есть пользователь видит стандартную светлую шапку
// и белое поле примерно на 600 мс, пока создаётся WebView2. Применять
// безрамочность после возврата из NewWithOptions поздно: вспышка уже была.
//
// Поэтому перехватываем создание окна хуком WH_CBT и правим стиль ДО того,
// как окно появится на экране: убираем заголовок и снимаем WS_VISIBLE.
// Окно при этом не скрываем: WebView2 не рендерит скрытое окно, и JS не
// выполняется — приложение осталось бы пустым.
const (
	whCBT         = 5
	hcbTCreateWnd = 3
	wsVisible     = 0x10000000
	swShow        = 5
	// Смещение поля style в структуре CREATESTRUCT (x64).
	// 4 указателя (32) + cy,cx,y,x (16) = 48.
	createstructStyleOffset = 48
)

var (
	procSetWindowsHookEx    = syscall.NewLazyDLL("user32.dll").NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = syscall.NewLazyDLL("user32.dll").NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = syscall.NewLazyDLL("user32.dll").NewProc("CallNextHookEx")
	procShowWindow          = syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow")
	procGetClassName        = syscall.NewLazyDLL("user32.dll").NewProc("GetClassNameW")
	procGetCurrentThreadId  = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")
	cbtHook                 uintptr
	hideUntilReady          bool
)

// cbtProc правит стиль окна в момент создания: без заголовка и невидимым.
func cbtProc(nCode, wParam, lParam uintptr) uintptr {
	if int32(nCode) == hcbTCreateWnd && lParam != 0 && hideUntilReady {
		// lParam указывает на CBT_CREATEWND, а не на CREATESTRUCT:
		// первое поле — указатель на CREATESTRUCT (LPCREATESTRUCT lpcs).
		lpcs := *(*uintptr)(unsafe.Pointer(lParam))
		if lpcs != 0 && isMainWindow(wParam) {
			// Стиль правим ДО появления окна: иначе пользователь видит
			// стандартную светлую шапку, пока создаётся WebView2 (~0.6 с).
			stylePtr := (*int32)(unsafe.Pointer(lpcs + createstructStyleOffset))
			st := *stylePtr
			st &^= wsCaption | wsSysMenu | wsMinimizeBox | wsMaximizeBox
			st |= wsThickFrame | wsBorder
			*stylePtr = st
		}
		if isMainWindow(wParam) {
			installSubclass(wParam)
		}
	}
	r, _, _ := procCallNextHookEx.Call(cbtHook, nCode, wParam, lParam)
	return r
}

// installCreateHook ставит хук на создание окон текущего потока.
// Возвращает функцию снятия хука.
func installCreateHook() func() {
	hideUntilReady = true
	tid, _, _ := procGetCurrentThreadId.Call()
	h, _, _ := procSetWindowsHookEx.Call(whCBT, syscall.NewCallback(cbtProc), 0, tid)
	cbtHook = h
	return func() {
		if cbtHook != 0 {
			procUnhookWindowsHookEx.Call(cbtHook)
			cbtHook = 0
		}
		hideUntilReady = false
	}
}

// showWindow показывает окно (SW_SHOW).
//
// Окно НЕ скрывается на время загрузки: WebView2 не рендерит скрытое окно,
// и JS не выполняется вовсе — приложение остаётся пустым. Поэтому боремся
// с вспышкой иначе: стиль без заголовка ставится до создания окна (хук), а
// фон заливается своим цветом (WM_ERASEBKGND).
func showWindow(hwnd uintptr) {
	if hwnd != 0 {
		procShowWindow.Call(hwnd, swShow)
	}
}

// ---------- Заливка фона окна ----------

const (
	wmEraseBkgnd = 0x0014
	// Цвет фона окна в COLORREF — 0x00BBGGRR (не RGB!).
	// --bg из ui/index.html: #0a101d -> R=0x0a, G=0x10, B=0x1d -> 0x001d100a.
	windowBgColor = 0x001d100a
)

var (
	procGetClientRectBg  = syscall.NewLazyDLL("user32.dll").NewProc("GetClientRect")
	procCreateSolidBrush = syscall.NewLazyDLL("gdi32.dll").NewProc("CreateSolidBrush")
	procFillRect         = syscall.NewLazyDLL("user32.dll").NewProc("FillRect")
	procDeleteObject     = syscall.NewLazyDLL("gdi32.dll").NewProc("DeleteObject")
)

// fillWindowBackground заливает клиентскую область цветом интерфейса.
//
// Зачем. Пока WebView2 создаётся и грузит HTML, окно уже видно, а содержимого
// ещё нет. Без своей заливки DefWindowProc красит его белым — на тёмно-синем
// интерфейсе это выглядит как вспышка. Цвет берём ровно тот же, что у body,
// поэтому переход к интерфейсу незаметен.
func fillWindowBackground(hwnd, hdc uintptr) {
	var cr rect
	procGetClientRectBg.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
	brush, _, _ := procCreateSolidBrush.Call(windowBgColor)
	if brush == 0 {
		return
	}
	defer procDeleteObject.Call(brush)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&cr)), brush)
}

// isMainWindow сообщает, что создаётся главное окно приложения.
//
// Зачем. WH_CBT ловит создание ЛЮБОГО окна в потоке. Первым приходит
// служебное окно WebView2, и его класс задан не строкой, а атомом
// (в lpszClass лежит маленькое число вроде 0xc176) — читать его как
// указатель нельзя, это падение. Поэтому имя класса спрашиваем у самой
// Windows через GetClassNameW: для главного окна библиотека регистрирует
// класс "webview" (RegisterClassEx в go-webview2).
func isMainWindow(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	var buf [64]uint16
	n, _, _ := procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return false
	}
	return strings.EqualFold(windows.UTF16ToString(buf[:n]), "webview")
}

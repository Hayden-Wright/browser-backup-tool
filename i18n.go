package main

import "strings"

// Lang — выбранный язык интерфейса.
type Lang int

const (
	EN Lang = iota
	RU
)

// defaultLang — язык по умолчанию (код для файла настроек).
// Английский основной: программа публичная, аудитория GitHub международная.
// Русский остаётся полным вторым языком и включается кнопкой в окне.
const defaultLang = "en"

// currentLang — текущий язык интерфейса.
var currentLang = EN

// s — таблица строк: ключ -> [английский, русский].
var s = map[string][2]string{
	// Общее
	"app_title": {"Browser Profile Backup", "Бэкап профилей браузеров"},
	"app_subtitle": {"Save and restore your browser profiles — locally, with no traces left in the system.",
		"Сохраняет и восстанавливает профили браузеров — локально, без следов в системе."},
	"language":       {"Language", "Язык"},
	"browsers_group": {"Browsers and profiles", "Браузеры и профили"},
	"custom_browser": {"Other browser (manual folder)", "Другой браузер (папка вручную)"},
	"add_folder":     {"Add folder…", "Добавить папку…"},
	"add_folder_title": {"Choose a profile folder of another browser",
		"Выберите папку профиля другого браузера"},
	"no_browsers": {"No supported browsers were found on this computer.",
		"На этом компьютере не найдены поддерживаемые браузеры."},
	"rescan":          {"Rescan", "Обновить список"},
	"backup_dir":      {"Backup folder", "Папка для бэкапа"},
	"change":          {"Change…", "Изменить…"},
	"open_folder":     {"Open folder", "Открыть папку"},
	"create_backup":   {"Create backup", "Создать бэкап"},
	"restore":         {"Restore", "Восстановить"},
	"view_log":        {"View log", "Открыть лог"},
	"log_group":       {"Log", "Журнал"},
	"ready":           {"Ready.", "Готово."},
	"select_all":      {"Select all", "Выбрать все"},
	"select_none":     {"Clear", "Снять выбор"},
	"default_profile": {"default", "основной"},

	// Статусы
	"st_scanning":    {"Scanning for browsers…", "Ищу установленные браузеры…"},
	"st_backing_up":  {"Creating backup…", "Создаю бэкап…"},
	"st_restoring":   {"Restoring…", "Восстанавливаю…"},
	"st_done":        {"Done.", "Готово."},
	"st_error":       {"Error.", "Ошибка."},
	"st_cancelled":   {"Cancelled.", "Отменено."},
	"st_copying":     {"Copying: %s", "Копирую: %s"},
	"st_safety":      {"Making a safety copy of the current profile…", "Делаю страховочную копию текущего профиля…"},
	"st_verifying":   {"Verifying integrity…", "Проверяю целостность…"},
	"st_finished_bk": {"Backup finished successfully.", "Бэкап успешно создан."},
	"st_finished_rs": {"Restore finished successfully.", "Восстановление успешно завершено."},

	// Предупреждения
	"warn_c_drive_title": {"Backup on drive C:", "Бэкап на диске C:"},
	"warn_c_drive": {"The backup folder is on drive C:.\n\nWhen you reinstall Windows, drive C: is erased and the backup will be lost.\n\nIt is strongly recommended to store the backup on another drive (for example D:).\n\nUse this folder anyway?",
		"Папка бэкапа находится на диске C:.\n\nПри переустановке Windows диск C: стирается, и бэкап будет потерян.\n\nНастоятельно рекомендуется хранить бэкап на другом диске (например, D:).\n\nВсё равно использовать эту папку?"},
	"browser_running_title": {"Browser is running", "Браузер запущен"},
	"browser_running": {"These browsers are currently running:\n\n%s\n\nA browser must be closed to back up or restore its profile — otherwise some files cannot be copied and the profile may be damaged.\n\nClose them and try again.",
		"Сейчас запущены:\n\n%s\n\nБраузер должен быть закрыт для создания бэкапа или восстановления — иначе часть файлов не скопируется, а профиль может повредиться.\n\nЗакройте их и попробуйте снова."},
	"no_selection_title":   {"Nothing selected", "Ничего не выбрано"},
	"no_selection":         {"Please select at least one browser profile.", "Выберите хотя бы один профиль браузера."},
	"no_dir_title":         {"Backup folder not set", "Папка бэкапа не задана"},
	"no_dir":               {"Please choose a folder for backups first.", "Сначала выберите папку для бэкапов."},
	"nothing_backup_title": {"No backups found", "Бэкапы не найдены"},
	"nothing_backup":       {"No backups were found in the selected folder.", "В выбранной папке не найдено ни одного бэкапа."},

	// Диалоги
	"choose_dir_title":      {"Choose a folder for backups", "Выберите папку для бэкапов"},
	"confirm_restore_title": {"Confirm restore", "Подтвердите восстановление"},
	"confirm_restore": {"The following profiles will be restored:\n\n%s\n\nYour current profiles will NOT be deleted — a safety copy will be made first.\n\nContinue?",
		"Будут восстановлены профили:\n\n%s\n\nТекущие профили НЕ удаляются — сначала будет сделана страховочная копия.\n\nПродолжить?"},
	"yes": {"Yes", "Да"},
	"no":  {"No", "Нет"},

	// Результаты
	"done_backup_title":  {"Backup complete", "Бэкап создан"},
	"done_backup_body":   {"Backup saved to:\n%s\n\n%s", "Бэкап сохранён в:\n%s\n\n%s"},
	"done_restore_title": {"Restore complete", "Восстановление завершено"},
	"done_restore_body": {"Profiles have been restored.\n\nA safety copy of your previous profiles was saved to:\n%s\n\nYou can delete it later if everything works fine.",
		"Профили восстановлены.\n\nСтраховочная копия прежних профилей сохранена в:\n%s\n\nПозже её можно удалить, если всё работает хорошо."},
	"error_title": {"Error", "Ошибка"},

	// Ошибки простым языком
	"err_copy":                {"Could not copy the file:\n%s\n\nReason: %s", "Не удалось скопировать файл:\n%s\n\nПричина: %s"},
	"err_no_space":            {"Not enough free disk space to create the backup.", "Недостаточно свободного места на диске для бэкапа."},
	"err_access":              {"Access denied. Try running the program as administrator, or close the browser.", "Доступ запрещён. Попробуйте запустить программу от имени администратора или закройте браузер."},
	"err_generic":             {"Something went wrong: %s", "Что-то пошло не так: %s"},
	"err_no_profiles_restore": {"There is nothing to restore for this browser in the backup.", "В бэкапе нет данных для восстановления этого браузера."},

	// Итоги в журнале
	"log_started":      {"=== %s started ===", "=== %s запущено ==="},
	"log_lang":         {"Language: %s", "Язык: %s"},
	"log_dir":          {"Backup folder: %s", "Папка бэкапа: %s"},
	"log_found":        {"Found browser: %s", "Найден браузер: %s"},
	"log_profile":      {"  profile: %s%s", "  профиль: %s%s"},
	"log_backup_one":   {"Backing up %s — %s", "Бэкап: %s — %s"},
	"log_restore_one":  {"Restoring %s — %s", "Восстановление: %s — %s"},
	"log_safety":       {"Safety copy: %s", "Страховочная копия: %s"},
	"log_total":        {"Total size: %s", "Общий размер: %s"},
	"log_copied":       {"Copied %s", "Скопировано %s"},
	"log_skipped_lock": {"Skipped locked file: %s", "Пропущен занятый файл: %s"},
	"log_warn":         {"Warning: %s", "Предупреждение: %s"},
	"log_error":        {"Error: %s", "Ошибка: %s"},
	"log_done":         {"Finished.", "Завершено."},
	"log_ready":        {"Ready. Backup folder: %s", "Готово к работе. Папка бэкапа: %s"},

	// Файл-инструкция внутри папки бэкапа (ПРОЧТИ_МЕНЯ.txt).
	// Пишется на языке, выбранном при создании бэкапа.
	"readme_browser":  {"Browser:      %s", "Браузер:      %s"},
	"readme_profile":  {"Profile:      %s", "Профиль:      %s"},
	"readme_created":  {"Created:      %s", "Создан:       %s"},
	"readme_windows":  {"Windows:      %s", "Windows:      %s"},
	"readme_computer": {"Computer:     %s", "Компьютер:    %s"},
	"readme_user":     {"User:         %s", "Пользователь: %s"},
	"readme_files":    {"Files:        %s", "Файлов:       %s"},
	"readme_size":     {"Size:         %s", "Размер:       %s"},
	"readme_howto": {"To restore: run the program, pick this backup and click Restore.\nThe browser must be closed.",
		"Чтобы восстановить: запустите программу, выберите этот бэкап\nи нажмите «Восстановить». Браузер должен быть закрыт."},
}

// T возвращает строку по ключу на текущем языке.
func T(key string) string {
	if v, ok := s[key]; ok {
		return v[currentLang]
	}
	return key
}

// Tf подставляет аргументы в строку по ключу.
func Tf(key string, args ...any) string {
	return sprintf(T(key), args...)
}

// langName — человекочитаемое имя языка.
func langName(l Lang) string {
	if l == RU {
		return "Русский"
	}
	return "English"
}

// parseLang разбирает код языка из настроек. Неизвестное или пустое значение
// даёт английский — он основной. Русский включается только явным "ru".
func parseLang(v string) Lang {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "ru", "russian", "рус":
		return RU
	case "en", "english", "англ":
		return EN
	}
	return EN
}

func langCode(l Lang) string {
	if l == EN {
		return "en"
	}
	return "ru"
}

// readmeFileName — имя файла-инструкции внутри папки бэкапа. Английский
// основной, поэтому и имя английское; русское остаётся для бэкапов,
// созданных на русском.
//
// Оба варианта надо пропускать при восстановлении (см. sourceItems): файл
// лежит в корне бэкапа и иначе был бы скопирован в профиль как «Root»-файл.
func readmeFileName(l Lang) string {
	if l == RU {
		return "ПРОЧТИ_МЕНЯ.txt"
	}
	return "README.txt"
}

// isReadmeFile сообщает, является ли имя файлом-инструкцией бэкапа (на любом
// языке) — такие файлы не восстанавливаются.
func isReadmeFile(name string) bool {
	return name == readmeFileName(EN) || name == readmeFileName(RU)
}

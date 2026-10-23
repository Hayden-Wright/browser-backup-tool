package main

import "testing"

// Английский — основной язык: он должен выбираться при первом запуске,
// когда файла настроек ещё нет.
func TestDefaultLanguageIsEnglish(t *testing.T) {
	if defaultLang != "en" {
		t.Fatalf("defaultLang = %q, want \"en\"", defaultLang)
	}
	if currentLang != EN {
		t.Fatalf("currentLang = %v, want EN", currentLang)
	}
	if got := loadSettings().Language; got != "en" {
		t.Fatalf("loadSettings().Language = %q, want \"en\"", got)
	}
}

// Неизвестное, пустое или испорченное значение не должно откатывать на русский.
func TestParseLangFallsBackToEnglish(t *testing.T) {
	cases := []struct {
		in   string
		want Lang
	}{
		{"", EN},
		{"en", EN},
		{"EN", EN},
		{"english", EN},
		{"  en  ", EN},
		{"de", EN},
		{"garbage", EN},
		{"ru", RU},
		{"RU", RU},
		{"russian", RU},
	}
	for _, c := range cases {
		if got := parseLang(c.in); got != c.want {
			t.Errorf("parseLang(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// Оба языка должны содержать одинаковый набор ключей: иначе переключение
// на русский даёт пустые подписи вместо текста.
func TestAllStringsHaveBothLanguages(t *testing.T) {
	for key, v := range s {
		if v[EN] == "" {
			t.Errorf("key %q has empty English string", key)
		}
		if v[RU] == "" {
			t.Errorf("key %q has empty Russian string", key)
		}
	}
}

// Файл-инструкция бэкапа не должен попадать в профиль при восстановлении —
// ни английский, ни русский вариант (старые бэкапы созданы на русском).
func TestReadmeFileNotRestored(t *testing.T) {
	for _, name := range []string{"README.txt", "ПРОЧТИ_МЕНЯ.txt"} {
		if !isReadmeFile(name) {
			t.Errorf("isReadmeFile(%q) = false, want true", name)
		}
	}
	// Обычный файл профиля не должен считаться инструкцией.
	if isReadmeFile("Bookmarks") {
		t.Error(`isReadmeFile("Bookmarks") = true, want false`)
	}
}

// Имя файла-инструкции следует языку: английский основной, поэтому и имя
// английское. Русский вариант сохраняется для совместимости со старыми бэкапами.
func TestReadmeFileNameFollowsLanguage(t *testing.T) {
	if got := readmeFileName(EN); got != "README.txt" {
		t.Errorf("readmeFileName(EN) = %q, want %q", got, "README.txt")
	}
	if got := readmeFileName(RU); got != "ПРОЧТИ_МЕНЯ.txt" {
		t.Errorf("readmeFileName(RU) = %q, want %q", got, "ПРОЧТИ_МЕНЯ.txt")
	}
}

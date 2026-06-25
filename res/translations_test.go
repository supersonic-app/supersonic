package res

import "testing"

func TestTranslationFilesExist(t *testing.T) {
	translationFiles := []string{
		"translations/zhHans.json",
		"translations/zhHant.json",
		"translations/en.json",
	}

	for _, fileName := range translationFiles {
		t.Run(fileName, func(t *testing.T) {
			if _, err := Translations.ReadFile(fileName); err != nil {
				t.Fatalf("Translations.ReadFile(%q) returned error: %v", fileName, err)
			}
		})
	}
}

func TestTranslationsInfoRegistration(t *testing.T) {
	tests := []struct {
		name                string
		translationFileName string
	}{
		{name: "zhHans", translationFileName: "zhHans.json"},
		{name: "zhHant", translationFileName: "zhHant.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for _, info := range TranslationsInfo {
				if info.Name == tt.name && info.TranslationFileName == tt.translationFileName {
					found = true
					break
				}
			}

			if !found {
				t.Fatalf("TranslationsInfo missing entry with Name=%q and TranslationFileName=%q", tt.name, tt.translationFileName)
			}
		})
	}
}

func TestTranslationsBCP47Locales(t *testing.T) {
	tests := []struct {
		name        string
		bcp47Locale string
	}{
		{name: "zhHans", bcp47Locale: "zh-Hans"},
		{name: "zhHant", bcp47Locale: "zh-Hant"},
		{name: "pt_BR", bcp47Locale: "pt-BR"},
		{name: "en", bcp47Locale: "en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for _, info := range TranslationsInfo {
				if info.Name == tt.name && info.BCP47Locale == tt.bcp47Locale {
					found = true
					break
				}
			}

			if !found {
				t.Fatalf("TranslationsInfo missing entry with Name=%q and BCP47Locale=%q", tt.name, tt.bcp47Locale)
			}
		})
	}
}

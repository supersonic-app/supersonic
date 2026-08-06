package res

import "testing"

func TestTranslationFilesUseBCP47Names(t *testing.T) {
	tests := []struct {
		name                string
		translationFileName string
	}{
		{name: "pt_BR", translationFileName: "pt-BR.json"},
		{name: "zhHans", translationFileName: "zh-Hans.json"},
		{name: "zhHant", translationFileName: "zh-Hant.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, info := range TranslationsInfo {
				if info.Name != tt.name {
					continue
				}
				if info.TranslationFileName != tt.translationFileName {
					t.Fatalf("TranslationFileName for %q = %q, want %q", tt.name, info.TranslationFileName, tt.translationFileName)
				}
				if _, err := Translations.ReadFile("translations/" + info.TranslationFileName); err != nil {
					t.Fatalf("Translations.ReadFile(%q) returned error: %v", info.TranslationFileName, err)
				}
				return
			}
			t.Fatalf("TranslationsInfo missing entry with Name=%q", tt.name)
		})
	}
}

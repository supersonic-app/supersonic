package res

import (
	"strings"
	"testing"

	"golang.org/x/text/language"
)

func TestTranslationFilesUseCanonicalBCP47Names(t *testing.T) {
	for _, info := range TranslationsInfo {
		t.Run(info.Name, func(t *testing.T) {
			localeName, ok := strings.CutSuffix(info.TranslationFileName, ".json")
			if !ok {
				t.Fatalf("TranslationFileName for %q = %q, want a .json file", info.Name, info.TranslationFileName)
			}
			tag, err := language.Parse(localeName)
			if err != nil {
				t.Fatalf("TranslationFileName for %q has invalid locale %q: %v", info.Name, localeName, err)
			}
			if got := tag.String(); got != localeName {
				t.Fatalf("TranslationFileName for %q = %q, want canonical locale %q", info.Name, localeName, got)
			}
			if _, err := Translations.ReadFile("translations/" + info.TranslationFileName); err != nil {
				t.Fatalf("Translations.ReadFile(%q) returned error: %v", info.TranslationFileName, err)
			}
		})
	}
}

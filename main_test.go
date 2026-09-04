package main

import (
	"encoding/json"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"github.com/supersonic-app/supersonic/res"
)

func TestConfiguredLanguageOverridesSystemLocale(t *testing.T) {
	sharedBefore := lang.L("Advanced")
	tr := configuredTranslation(t, "zhHant")
	content, err := res.Translations.ReadFile("translations/" + tr.TranslationFileName)
	if err != nil {
		t.Fatal(err)
	}
	if err := addTranslationsForConfiguredLanguage(content, tr); err != nil {
		t.Fatal(err)
	}
	if got, want := lang.L("Settings"), "設定"; got != want {
		t.Fatalf("lang.L(\"Settings\") = %q, want %q", got, want)
	}
	if got := lang.L("Advanced"); got != sharedBefore {
		t.Fatalf("lang.L(\"Advanced\") = %q, want existing Fyne translation %q", got, sharedBefore)
	}
}

func TestConfiguredLanguageRegistrationPlan(t *testing.T) {
	tests := []struct {
		name      string
		file      string
		preferred []string
		want      []fyne.Locale
	}{
		{"Chinese region to Traditional", "zh-Hant.json", []string{"zh-CN"}, []fyne.Locale{"zh-Hant", "zh-CN", "zh", "zh-Hans"}},
		{"Traditional region to Simplified", "zh-Hans.json", []string{"zh-TW"}, []fyne.Locale{"zh-Hans", "zh-TW", "zh", "zh-Hant"}},
		{"Portuguese with multiple preferences", "pt-BR.json", []string{"de-DE", "fr-FR"}, []fyne.Locale{"pt-BR", "de-DE", "de", "de-Latn", "fr-FR", "fr", "fr-Latn"}},
		{"Explicit non-default script", "pt-BR.json", []string{"uz-Cyrl-UZ"}, []fyne.Locale{"pt-BR", "uz-UZ", "uz", "uz-Cyrl"}},
		{"Invalid preference", "pt-BR.json", []string{"not_a_locale_!", "pt-PT"}, []fyne.Locale{"pt-BR", "pt-PT", "pt", "pt-Latn"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := configuredLanguageRegistrationPlan(tt.file, tt.preferred)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("registration locales = %v, want %v", got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("registration locale %d = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

func TestTranslationAliasContent(t *testing.T) {
	content := []byte(`{"Settings":"Configurações","Advanced":"Advanced","Unknown":"Unknown"}`)
	alias, err := translationAliasContent(content, func(key string) string {
		if key == "Advanced" {
			return "Avançado"
		}
		return key
	})
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]string
	if err := json.Unmarshal(alias, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"Settings": "Configurações", "Advanced": "Avançado"}
	if len(got) != len(want) {
		t.Fatalf("alias content = %v, want %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("alias content[%q] = %q, want %q", key, got[key], value)
		}
	}
}

func configuredTranslation(t *testing.T, name string) res.TranslationInfo {
	t.Helper()
	for _, tr := range res.TranslationsInfo {
		if tr.Name == name {
			return tr
		}
	}
	t.Fatalf("TranslationsInfo missing entry with Name=%q", name)
	return res.TranslationInfo{}
}

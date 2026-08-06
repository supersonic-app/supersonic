package main

import (
	"testing"

	"fyne.io/fyne/v2/lang"
	"github.com/supersonic-app/supersonic/res"
)

func TestConfiguredLanguageOverridesSystemLocale(t *testing.T) {
	content, err := res.Translations.ReadFile("translations/zh-Hant.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := addTranslationsForConfiguredLanguage(content); err != nil {
		t.Fatal(err)
	}
	if got, want := lang.L("Settings"), "設定"; got != want {
		t.Fatalf("lang.L(\"Settings\") = %q, want %q", got, want)
	}
}

package main

import (
	"testing"

	"fyne.io/fyne/v2/lang"
	"github.com/supersonic-app/supersonic/res"
)

func TestConfiguredLanguageOverridesSystemLocale(t *testing.T) {
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
}

func TestConfiguredTraditionalChineseDoesNotOverwriteSimplifiedFyneTranslations(t *testing.T) {
	t.Setenv("LANGUAGE", "")
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")
	if locale := lang.SystemLocale(); locale.String() != "zh-CN" {
		t.Skipf("cannot force simplified Chinese locale on this platform: %s", locale)
	}

	tr := configuredTranslation(t, "zhHant")
	content, err := res.Translations.ReadFile("translations/" + tr.TranslationFileName)
	if err != nil {
		t.Fatal(err)
	}
	if err := addTranslationsForConfiguredLanguage(content, tr); err != nil {
		t.Fatal(err)
	}

	for key, want := range map[string]string{
		"Advanced": "高级",
		"Error":    "错误",
		"Save":     "保存",
	} {
		if got := lang.L(key); got != want {
			t.Errorf("lang.L(%q) = %q, want %q", key, got, want)
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

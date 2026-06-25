package res

import "embed"

//go:embed translations
var Translations embed.FS

type TranslationInfo struct {
	Name                string
	DisplayName         string
	TranslationFileName string
	BCP47Locale         string // BCP47 locale tag for Fyne i18n matching
}

var TranslationsInfo = []TranslationInfo{
	{Name: "de", DisplayName: "Deutsch", TranslationFileName: "de.json", BCP47Locale: "de"},
	{Name: "el", DisplayName: "Ελληνικά", TranslationFileName: "el.json", BCP47Locale: "el"},
	{Name: "en", DisplayName: "English", TranslationFileName: "en.json", BCP47Locale: "en"},
	{Name: "es", DisplayName: "Español", TranslationFileName: "es.json", BCP47Locale: "es"},
	{Name: "fr", DisplayName: "Français", TranslationFileName: "fr.json", BCP47Locale: "fr"},
	{Name: "it", DisplayName: "Italiano", TranslationFileName: "it.json", BCP47Locale: "it"},
	{Name: "ja", DisplayName: "日本語", TranslationFileName: "ja.json", BCP47Locale: "ja"},
	{Name: "ko", DisplayName: "한국어", TranslationFileName: "ko.json", BCP47Locale: "ko"},
	{Name: "nl", DisplayName: "Nederlands", TranslationFileName: "nl.json", BCP47Locale: "nl"},
	{Name: "pl", DisplayName: "Polski", TranslationFileName: "pl.json", BCP47Locale: "pl"},
	{Name: "pt_BR", DisplayName: "Português (BR)", TranslationFileName: "pt_BR.json", BCP47Locale: "pt-BR"},
	{Name: "ro", DisplayName: "Română", TranslationFileName: "ro.json", BCP47Locale: "ro"},
	{Name: "ru", DisplayName: "Русский", TranslationFileName: "ru.json", BCP47Locale: "ru"},
	{Name: "tr", DisplayName: "Türkçe", TranslationFileName: "tr.json", BCP47Locale: "tr"},
	{Name: "zhHans", DisplayName: "中文", TranslationFileName: "zhHans.json", BCP47Locale: "zh-Hans"},
	{Name: "zhHant", DisplayName: "中文 (trad.)", TranslationFileName: "zhHant.json", BCP47Locale: "zh-Hant"},
}

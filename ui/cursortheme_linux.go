//go:build linux

package ui

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// SetCursorThemeEnvIfMissing works around GLFW's Wayland backend only being able
// to pick up the system cursor theme/size via the XCURSOR_THEME and XCURSOR_SIZE
// env vars, which modern desktop environments (GNOME, KDE Plasma 6+) no longer
// set for Wayland sessions. If the vars aren't already set, this snoops on the
// mechanisms GNOME and KDE Plasma use to keep GTK apps in sync, and sets
// them so GLFW's cursor theme loading has something to find.
func SetCursorThemeEnvIfMissing() {
	if os.Getenv("XCURSOR_THEME") == "" {
		if theme := gnomeCursorSetting("cursor-theme"); theme != "" {
			os.Setenv("XCURSOR_THEME", theme)
		} else if theme := gtkCursorSetting("gtk-cursor-theme-name"); theme != "" {
			os.Setenv("XCURSOR_THEME", theme)
		}
	}
	if os.Getenv("XCURSOR_SIZE") == "" {
		if size := gnomeCursorSetting("cursor-size"); isPositiveInt(size) {
			os.Setenv("XCURSOR_SIZE", size)
		} else if size := gtkCursorSetting("gtk-cursor-theme-size"); isPositiveInt(size) {
			os.Setenv("XCURSOR_SIZE", size)
		}
	}
}

func isPositiveInt(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

// gnomeCursorSetting reads a key from the org.gnome.desktop.interface gsettings
// schema, which GNOME (and several other DEs that share its settings backend) uses
// to store the cursor theme and size.
func gnomeCursorSetting(key string) string {
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", key).Output()
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(string(out)), "'")
}

// gtkCursorSetting reads a key from gtk-4.0/settings.ini, which KDE Plasma (and
// other non-GNOME desktops) keeps updated with the system cursor theme/size for
// the benefit of GTK apps, since there is no cross-desktop standard way for an
// arbitrary Wayland client to query it directly.
func gtkCursorSetting(key string) string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	f, err := os.Open(filepath.Join(configDir, "gtk-4.0", "settings.ini"))
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

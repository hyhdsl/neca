package web

import "testing"

func TestSettingsTemplatesRegistered(t *testing.T) {
	templates, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	for _, name := range []string{"settings", "settings_security", "settings_about"} {
		if templates[name] == nil {
			t.Fatalf("template %q is not registered", name)
		}
	}
}

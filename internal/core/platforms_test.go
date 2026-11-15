package core

import "testing"

func TestNormalizePlatforms(t *testing.T) {
	got, err := NormalizePlatforms([]string{"win,android", "mac", "windows"})
	if err != nil {
		t.Fatalf("NormalizePlatforms error = %v", err)
	}
	want := []string{"windows", "android", "macos"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if _, err := NormalizePlatforms([]string{"solaris"}); err == nil {
		t.Fatal("未知平台应报错")
	}
}

func TestParseInstallers(t *testing.T) {
	none, err := ParseInstallers("none")
	if err != nil || len(none) != 0 {
		t.Fatalf("none = %v, %v", none, err)
	}
	all, err := ParseInstallers("all")
	if err != nil || !all.Enabled(PlatformWindows) || !all.Enabled(PlatformLinux) || !all.Enabled(PlatformMacOS) {
		t.Fatalf("all = %v, %v", all, err)
	}
	win, err := ParseInstallers("windows")
	if err != nil || !win.Enabled(PlatformWindows) || win.Enabled(PlatformLinux) {
		t.Fatalf("windows = %v, %v", win, err)
	}
	if _, err := ParseInstallers("android"); err == nil {
		t.Fatal("Android 不支持一键脚本，应报错")
	}
}

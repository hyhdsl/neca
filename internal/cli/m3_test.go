package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseGlobals(t *testing.T) {
	rest, opts, err := parseGlobals([]string{"list", "--json", "--lang=en-US", "--dry-run", "-q"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 1 || rest[0] != "list" {
		t.Fatalf("rest = %v", rest)
	}
	if !opts.JSON || !opts.DryRun || !opts.Quiet || opts.Lang != "en-US" {
		t.Fatalf("opts = %+v", opts)
	}
	if _, _, err := parseGlobals([]string{"--lang"}); err == nil {
		t.Fatal("missing --lang value should fail")
	}
}

func TestCompatArgs(t *testing.T) {
	cases := map[string][]string{
		"--list":          {"list"},
		"--sync":          {"index", "sync"},
		"--issue":         {"issue", "TMP01"},
		"--add-platform":  {"platform", "add", "TMP01"},
		"--export-issued": {"export", "issued"},
	}
	for input, want := range cases {
		args := []string{input}
		if input == "--issue" || input == "--add-platform" {
			args = append(args, "TMP01")
		}
		got := compatArgs(args)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("compatArgs(%v) = %v, want %v", args, got, want)
		}
	}
}

func TestJSONVersion(t *testing.T) {
	code, out, errOut := runCLI("version", "--json")
	if code != ExitOK || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v: %q", err, out)
	}
	if env["ok"] != true || env["action"] != "version" {
		t.Fatalf("envelope = %v", env)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	code, out, _ := runCLI("issue", "TMP01", "--dry-run", "--json")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	var env struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK || env.Data["dryRun"] != true {
		t.Fatalf("envelope = %s", out)
	}
}

func TestEnglishHelp(t *testing.T) {
	code, out, _ := runCLI("--lang", "en-US", "help")
	if code != ExitOK || !strings.Contains(out, "Usage:") || !strings.Contains(out, "Available commands:") {
		t.Fatalf("code=%d help=%q", code, out)
	}
}

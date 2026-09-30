package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultsValid(t *testing.T) {
	for _, s := range Sections {
		for _, b := range s.Bindings {
			for _, k := range b.Keys {
				if _, err := NormalizeKey(k); err != nil {
					t.Errorf("default %s.%s: %v", s.Name, b.Action, err)
				}
			}
		}
	}
	for _, c := range Colors {
		if _, err := normalizeColor(c.Default); err != nil {
			t.Errorf("default colour %s: %v", c.Name, err)
		}
	}
	if _, err := Parse("", "empty"); err != nil {
		t.Errorf("defaults clash: %v", err)
	}
}

func TestExampleRoundTrips(t *testing.T) {
	got, err := Parse(Example(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, Default()) {
		t.Error("the written example does not load back to the defaults")
	}
}

func TestOverrides(t *testing.T) {
	c, err := Parse(`
[colors]
accent = "#FF0000"
muted = "8"

[keys.list]
quit = ["Q", "ctrl+Q"]
details = ["Enter"]
open = ["o"]
[keys.details]
page_down = ["space"]
`, "t")
	if err != nil {
		t.Fatal(err)
	}
	if c.Colors["accent"] != "#ff0000" || c.Colors["muted"] != "8" || c.Colors["text"] != "#cfd8e3" {
		t.Errorf("colors %v", c.Colors)
	}
	if got := c.Keys["list"]["quit"]; !reflect.DeepEqual(got, []string{"Q", "ctrl+q"}) {
		t.Errorf("quit = %q", got)
	}
	if got := c.Keys["list"]["details"]; !reflect.DeepEqual(got, []string{"enter"}) {
		t.Errorf("details = %q", got)
	}
	if got := c.Keys["details"]["page_down"]; !reflect.DeepEqual(got, []string{" "}) {
		t.Errorf("page_down = %q", got)
	}
	if got := c.Keys["list"]["new"]; !reflect.DeepEqual(got, []string{"n"}) {
		t.Errorf("untouched action changed: %q", got)
	}
}

func TestMistakesAreReported(t *testing.T) {
	cases := map[string]string{
		"[colors]\naccnet = \"#fff\"":       `unknown colour "accnet"`,
		"[colors]\naccent = \"purple\"":     `"purple" is not a colour`,
		"[keys.list]\nquit = [\"hyper+q\"]": `unknown key "hyper+q"`,
		"[keys.list]\nqiut = [\"q\"]":       `unknown action "qiut"`,
		"[keys.lsit]\nquit = [\"q\"]":       `unknown section "lsit"`,
		"[keys.list]\nrename = [\"n\"]":     `"n" is bound to both`,
		"[colours]\naccent = \"#fff\"":      `unknown setting "colours`,
		"[colors\n":                         "t:",
	}
	for text, want := range cases {
		_, err := Parse(text, "t")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) error %v, want it to mention %s", text, err, want)
		}
	}
}

func TestLoadAndWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asp", "config.toml")
	if c, err := Load(path); err != nil || !reflect.DeepEqual(c, Default()) {
		t.Errorf("missing file: %v", err)
	}
	if err := WriteExample(path); err != nil {
		t.Fatal(err)
	}
	if err := WriteExample(path); err == nil {
		t.Error("WriteExample overwrote an existing file")
	}
	if c, err := Load(path); err != nil || !reflect.DeepEqual(c, Default()) {
		t.Errorf("written file: %v", err)
	}
	if err := os.WriteFile(path, []byte("[colors]\ntext = \"#123456\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, _ := Load(path); c.Colors["text"] != "#123456" {
		t.Errorf("text = %q", c.Colors["text"])
	}
}

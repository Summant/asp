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
		"[colors]\naccnet = \"#fff\"":                  `unknown colour "accnet"`,
		"[colors]\naccent = \"purple\"":                `"purple" is not a colour`,
		"[keys.list]\nquit = [\"hyper+q\"]":            `unknown key "hyper+q"`,
		"[keys.list]\nqiut = [\"q\"]":                  `unknown action "qiut"`,
		"[keys.lsit]\nquit = [\"q\"]":                  `unknown section "lsit"`,
		"[keys.list]\nrename = [\"n\"]\nnew = [\"n\"]": `"n" is bound to both new and rename; new keeps it`,
		"[colours]\naccent = \"#fff\"":                 `unknown setting "colours`,
		"[colors\n":                                    "t:",
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

// A key the user chose beats a default that wants it: a config written for
// one version keeps working when a later version adds an action whose
// default key the user already uses.
func TestUserKeysBeatDefaults(t *testing.T) {
	c, err := Parse("[keys.list]\nrename = [\"n\"]\n", "t")
	if err != nil {
		t.Fatalf("a user key colliding with a default is not a mistake: %v", err)
	}
	if got := c.Keys["list"]["rename"]; !reflect.DeepEqual(got, []string{"n"}) {
		t.Errorf("rename = %q", got)
	}
	if got := c.Keys["list"]["new"]; len(got) != 0 {
		t.Errorf("new kept n: %q", got)
	}
}

// Problems never stop asp: the Config is still usable, with every valid
// setting applied.
func TestProblemsAreNotFatal(t *testing.T) {
	c, err := Parse("[colors]\naccent = \"#ff0000\"\nnope = \"#fff\"\n[keys.list]\nquit = [\"Q\"]\nfuture_action = [\"z\"]\n", "t")
	if err == nil {
		t.Fatal("expected problems")
	}
	if c.Colors["accent"] != "#ff0000" || !reflect.DeepEqual(c.Keys["list"]["quit"], []string{"Q"}) {
		t.Error("valid settings were dropped because of an invalid one")
	}
	c, err = Parse("[colors\n", "t")
	if err == nil || !reflect.DeepEqual(c, Default()) {
		t.Error("a file that does not parse should give the defaults and a problem")
	}
}

// A config written by an older version, with personal changes.
const oldConfig = `# my asp config
[colors]
accent        = "#ff0000"  # changed
secondary     = "#8ec9ff"

# the session list
[keys.list]
open         = ["enter"]
quit         = ["Q"]
rename       = ["p"]    # took p before group_color existed

[keys.details]
back         = ["esc"]
`

func TestAddMissingKeepsEverything(t *testing.T) {
	out, added, err := AddMissing(oldConfig)
	if err != nil {
		t.Fatal(err)
	}
	// Every original line is still there, unchanged.
	for _, l := range strings.Split(strings.TrimSpace(oldConfig), "\n") {
		if !strings.Contains(out, l) {
			t.Errorf("line lost or changed: %q", l)
		}
	}
	// Same configuration before and after.
	before, _ := Parse(oldConfig, "before")
	after, err := Parse(out, "after")
	if err != nil {
		t.Fatalf("updated file has problems: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(before, after) {
		t.Error("adding the missing settings changed the configuration")
	}
	// What was missing is now written out, in its own section.
	for _, want := range []string{"colors.group", "keys.list.tab_add", "keys.list.group_color", "keys.finder.open"} {
		if !strings.Contains(strings.Join(added, " "), want) {
			t.Errorf("%s not added", want)
		}
	}
	if strings.Count(out, "[keys.list]") != 1 || strings.Count(out, "[colors]") != 1 {
		t.Error("a table was duplicated")
	}
	// group_color's default p gave way to the user's rename = ["p"]; it is
	// written as it is, without p, so nothing starts clashing.
	if !strings.Contains(out, `group_color  = []`) {
		t.Errorf("group_color written with a key that clashes:\n%s", out)
	}
	// Running it again adds nothing.
	if _, again, err := AddMissing(out); err != nil || len(again) != 0 {
		t.Errorf("second run: %v, %v", again, err)
	}
	// A complete file needs nothing.
	if _, none, err := AddMissing(Example()); err != nil || len(none) != 0 {
		t.Errorf("example: %v, %v", none, err)
	}
}

func TestUpdateFileKeepsABackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(oldConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := UpdateFile(path)
	if err != nil || len(added) == 0 {
		t.Fatalf("added %v, %v", added, err)
	}
	if b, _ := os.ReadFile(path + ".bak"); string(b) != oldConfig {
		t.Error("no backup of the original")
	}
	if _, err := UpdateFile(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Error("updating a missing file should fail")
	}
	if err := os.WriteFile(path, []byte("[colors\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateFile(path); err == nil {
		t.Error("a broken file should be left alone")
	}
	if b, _ := os.ReadFile(path); string(b) != "[colors\n" {
		t.Error("a broken file was changed")
	}
}

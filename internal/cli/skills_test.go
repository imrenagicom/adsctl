package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runSkills(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(context.Background(), args, &bytes.Buffer{}, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestSkillsIndexListsEverySkill(t *testing.T) {
	code, out, errOut := runSkills(t, "--skills")
	if code != 0 || errOut != "" {
		t.Fatalf("%d %s", code, errOut)
	}
	dirs, err := filepath.Glob("../../skills/*/SKILL.md")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no source skills: %v", err)
	}
	for _, skill := range dirs {
		name := filepath.Base(filepath.Dir(skill))
		if !strings.Contains(out, "\n## "+name+"\n") || !strings.Contains(out, "`adsctl --skills "+name+"`") {
			t.Fatalf("index misses %s:\n%s", name, out)
		}
		refs, _ := filepath.Glob(filepath.Join(filepath.Dir(skill), "references", "*.md"))
		for _, ref := range refs {
			want := "`adsctl --skills " + name + "/references/" + strings.TrimSuffix(filepath.Base(ref), ".md") + "`"
			if !strings.Contains(out, want) {
				t.Fatalf("index misses %s", want)
			}
		}
	}
	if !strings.Contains(out, "Use when") {
		t.Fatal("index should include skill descriptions")
	}
}

func TestSkillsOutputMatchesSourceFiles(t *testing.T) {
	files, _ := filepath.Glob("../../skills/*/SKILL.md")
	refs, _ := filepath.Glob("../../skills/*/references/*.md")
	for _, file := range append(files, refs...) {
		want, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel("../../skills", file)
		arg := filepath.ToSlash(strings.TrimSuffix(rel, ".md"))
		if filepath.Base(file) == "SKILL.md" {
			arg = filepath.Base(filepath.Dir(file))
		}
		for _, a := range []string{arg, arg + ".md"} {
			if strings.HasSuffix(a, "/SKILL.md") {
				continue
			}
			code, out, errOut := runSkills(t, "--skills", a)
			if code != 0 || errOut != "" || out != string(want) {
				t.Fatalf("%s: code %d stderr %s; output differs from %s", a, code, errOut, file)
			}
		}
	}
}

func TestSkillsUnknownIsStructured(t *testing.T) {
	for _, name := range []string{"nope", "adsctl/references/nope", "../go.mod", "adsctl/../../go.mod", "adsctl/agents/openai.yaml"} {
		code, out, errOut := runSkills(t, "--skills", name)
		if code != 2 || out != "" {
			t.Fatalf("%s: %d %q", name, code, out)
		}
		var env struct {
			OK    bool                  `json:"ok"`
			Error struct{ Code string } `json:"error"`
		}
		if err := json.Unmarshal([]byte(errOut), &env); err != nil || env.OK || env.Error.Code != "SKILL_UNKNOWN" {
			t.Fatalf("%s: %v %s", name, err, errOut)
		}
	}
}

func TestRootRejectsUnknownCommandAndExtraSkillArgs(t *testing.T) {
	for _, args := range [][]string{{"nope"}, {"--skills", "adsctl", "adsctl-setup"}} {
		code, out, errOut := runSkills(t, args...)
		if code != 2 || out != "" || !strings.Contains(errOut, `"USAGE"`) {
			t.Fatalf("%v: %d %q %s", args, code, out, errOut)
		}
	}
	if code, out, _ := runSkills(t, "--help"); code != 0 || !strings.Contains(out, "--skills") {
		t.Fatal("root help should mention --skills")
	}
	if code, out, _ := runSkills(t, "docs"); code != 0 || !strings.HasPrefix(out, skillsHint) || !strings.Contains(out, "## `adsctl commands") {
		t.Fatal("docs should point to --skills before the command reference")
	}
}

package cli

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"adsctl"
	"adsctl/internal/output"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

// skillsHint points readers of command lists to the bundled workflows.
const skillsHint = "Step-by-step agent workflows are bundled with this binary: run `adsctl --skills` to list them and `adsctl --skills NAME` to read one.\n\n"

// skillsFS is the skills/ tree embedded at build time.
var skillsFS fs.FS = adsctl.Skills

type skillInfo struct {
	Name, Description string
	References        []string
}

// rootArgs accepts one positional argument only with --skills and otherwise
// keeps Cobra's unknown-command error, including its suggestions.
func rootArgs(skills *bool) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if *skills {
			return cobra.MaximumNArgs(1)(c, args)
		}
		if len(args) == 0 {
			return nil
		}
		msg := fmt.Sprintf("unknown command %q for %q", args[0], c.CommandPath())
		if c.SuggestionsMinimumDistance <= 0 {
			c.SuggestionsMinimumDistance = 2
		}
		if s := c.SuggestionsFor(args[0]); len(s) > 0 {
			msg += "\n\nDid you mean this?\n\t" + strings.Join(s, "\n\t") + "\n"
		}
		return output.Fail("USAGE", msg, 2)
	}
}

// printSkills writes the skill index, or one skill or reference file, as Markdown.
func printSkills(w io.Writer, args []string) error {
	if len(args) == 0 {
		return writeSkillIndex(w)
	}
	body, err := readSkill(args[0])
	if err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

func listSkills() ([]skillInfo, error) {
	entries, err := fs.ReadDir(skillsFS, "skills")
	if err != nil {
		return nil, err
	}
	var skills []skillInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		body, err := fs.ReadFile(skillsFS, path.Join("skills", e.Name(), "SKILL.md"))
		if err != nil {
			continue
		}
		info := skillInfo{Name: e.Name()}
		var front struct{ Name, Description string }
		if raw, ok := frontmatter(body); ok && yaml.Unmarshal(raw, &front) == nil {
			if front.Name != "" {
				info.Name = front.Name
			}
			info.Description = front.Description
		}
		refs, _ := fs.Glob(skillsFS, path.Join("skills", e.Name(), "references", "*.md"))
		for _, ref := range refs {
			info.References = append(info.References, strings.TrimSuffix(strings.TrimPrefix(ref, "skills/"), ".md"))
		}
		sort.Strings(info.References)
		skills = append(skills, info)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

func frontmatter(body []byte) ([]byte, bool) {
	rest, ok := bytes.CutPrefix(body, []byte("---\n"))
	if !ok {
		return nil, false
	}
	raw, _, ok := bytes.Cut(rest, []byte("\n---"))
	return raw, ok
}

func writeSkillIndex(w io.Writer) error {
	skills, err := listSkills()
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# adsctl bundled skills\n\n")
	b.WriteString("Agent workflows bundled with this binary. These are the same skills packaged for the Codex, Claude Code, and Google Antigravity plugins. Print one with `adsctl --skills NAME`; print a reference file with `adsctl --skills NAME/references/FILE`. No configuration, credentials, or network access is used.\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "\n## %s\n\n%s\n\nRead: `adsctl --skills %s`\n", s.Name, s.Description, s.Name)
		if len(s.References) > 0 {
			b.WriteString("\nReferences:\n")
			for _, ref := range s.References {
				fmt.Fprintf(&b, "- `adsctl --skills %s`\n", ref)
			}
		}
	}
	_, err = io.WriteString(w, b.String())
	return err
}

// readSkill resolves NAME to skills/NAME/SKILL.md and NAME/references/FILE
// (with or without .md) to the matching reference file.
func readSkill(arg string) ([]byte, error) {
	name := strings.TrimSuffix(strings.Trim(arg, "/"), ".md")
	file := name + ".md"
	if !strings.Contains(name, "/") {
		file = path.Join(name, "SKILL.md")
	}
	if fs.ValidPath(file) {
		if body, err := fs.ReadFile(skillsFS, path.Join("skills", file)); err == nil {
			return body, nil
		}
	}
	e := output.Fail("SKILL_UNKNOWN", fmt.Sprintf("No bundled skill or reference named %q.", arg), 2)
	e.SuggestedAction = "Run adsctl --skills to list bundled skills and references."
	return nil, e
}

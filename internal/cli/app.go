// Package cli implements adsctl's Cobra command interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"adsctl/internal/output"
	"github.com/spf13/cobra"
)

// version is set by release builds with -ldflags "-X adsctl/internal/cli.version=...".
var version = "dev"

type Options struct {
	Config, Profile, Account, APIVersion, Output, Color string
	Timeout, RequestTimeout                             time.Duration
	Verbose                                             bool
}
type App struct {
	baseURL    string
	httpClient *http.Client
	started    bool

	In       io.Reader
	Out, Err io.Writer
	opts     Options
	meta     map[string]any
}

func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	a := &App{In: in, Out: out, Err: errOut}
	root := a.Root()
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	if err := root.ExecuteContext(ctx); err != nil {
		e := output.AsError(err)
		var typed *output.Error
		if !errors.As(err, &typed) && !a.started {
			e = output.Fail("USAGE", err.Error(), 2)
		}
		env := output.New(nil, a.meta)
		env.OK = false
		env.Error = e
		_ = output.Write(errOut, "json", env)
		return e.Exit
	}
	return 0
}
func (a *App) emit(cmd *cobra.Command, data any) error {
	meta := map[string]any{"command": cmd.CommandPath(), "complete": true}
	for k, v := range a.meta {
		meta[k] = v
	}
	env := output.New(data, meta)
	if rows, ok := data.([]any); ok && len(rows) == 0 {
		if q, ok := meta["query"].(url.Values); ok {
			env.Warnings = append(env.Warnings, output.Warning{Code: "NO_ROWS", Message: "No Insights rows returned; this does not establish zero performance or metric availability."})
			breakdowns := q.Get("breakdowns")
			if strings.Contains(breakdowns, "impression_device") || strings.Contains(breakdowns, "hourly_stats_aggregated_by_audience_time_zone") || strings.Contains(breakdowns, "frequency_value") {
				env.Warnings = append(env.Warnings, output.Warning{Code: "BREAKDOWN_ACCESS", Message: "Some accounts require opt-in for this synchronous breakdown; check access or use an explicit async Insights job."})
			}
		}
	}
	return output.Write(a.Out, a.opts.Output, env)
}
func group(use, short string) *cobra.Command {
	c := &cobra.Command{Use: use, Short: short}
	c.RunE = func(c *cobra.Command, _ []string) error {
		_ = c.Help()
		return output.Fail("USAGE", "Choose a subcommand.", 2)
	}
	return c
}
func action(use, short, effect string, run func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, Annotations: map[string]string{"effect": effect}, RunE: run}
}
func (a *App) Root() *cobra.Command {
	root := &cobra.Command{Use: "adsctl", Short: "Manage advertising across platforms with an agent-friendly CLI", SilenceUsage: true, SilenceErrors: true}
	var skills bool
	root.Args = rootArgs(&skills)
	root.Example = "  adsctl --skills                  # list bundled agent skills\n  adsctl --skills adsctl           # print one skill as Markdown\n  adsctl commands --platform meta  # list implemented commands"
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if skills {
			return printSkills(a.Out, args)
		}
		return cmd.Help()
	}
	root.Flags().BoolVar(&skills, "skills", false, "Print bundled agent skills as Markdown; add NAME or NAME/references/FILE for one file")
	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		a.started = true
		if a.opts.Output != "json" && a.opts.Output != "jsonl" && a.opts.Output != "table" {
			return output.Fail("USAGE", "Output must be json, jsonl or table.", 2)
		}
		if a.opts.Color != "auto" && a.opts.Color != "always" && a.opts.Color != "never" {
			return output.Fail("USAGE", "Color must be auto, always or never.", 2)
		}
		if a.opts.Timeout <= 0 || a.opts.RequestTimeout <= 0 {
			return output.Fail("USAGE", "Timeouts must be positive.", 2)
		}
		return nil
	}
	f := root.PersistentFlags()
	f.StringVar(&a.opts.Config, "config", "", "Configuration file (default: $ADSCTL_HOME/config.yaml or ~/.adsctl/config.yaml)")
	f.StringVar(&a.opts.Profile, "profile", "", "Credential profile")
	f.StringVar(&a.opts.Account, "account", "", "Ad account ID")
	f.StringVar(&a.opts.APIVersion, "api-version", "", "Meta API version override")
	f.StringVar(&a.opts.Output, "output", "json", "Output: json, jsonl, table")
	f.StringVar(&a.opts.Color, "color", "auto", "Color preference (tables currently use no ANSI color)")
	f.DurationVar(&a.opts.Timeout, "timeout", 2*time.Minute, "Total operation deadline")
	f.DurationVar(&a.opts.RequestTimeout, "request-timeout", 30*time.Second, "Per-request deadline")
	f.BoolVar(&a.opts.Verbose, "verbose", false, "Redacted request diagnostics on stderr")
	root.AddCommand(action("version", "Print build version", "local", func(c *cobra.Command, _ []string) error { return a.emit(c, map[string]any{"version": version}) }))
	completion := action("completion SHELL", "Generate shell completion", "local", func(c *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return root.GenBashCompletion(a.Out)
		case "zsh":
			return root.GenZshCompletion(a.Out)
		case "fish":
			return root.GenFishCompletion(a.Out, true)
		case "powershell":
			return root.GenPowerShellCompletion(a.Out)
		}
		return output.Fail("USAGE", "Shell must be bash, zsh, fish or powershell.", 2)
	})
	completion.Args = cobra.ExactArgs(1)
	root.AddCommand(completion)
	a.addConfig(root)
	a.addMeta(root)
	annotateHelp(root)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return output.Fail("USAGE", fmt.Sprint(err), 2) })
	return root
}

func annotateHelp(root *cobra.Command) {
	for _, c := range root.Commands() {
		annotateHelp(c)
	}
	if root.Annotations["effect"] == "" {
		return
	}
	if root.Name() == "docs" || root.Name() == "completion" {
		return
	}
	effect := root.Annotations["effect"]
	root.Long = root.Short + ".\n\nEffect: " + effect + ". Data defaults to a versioned JSON envelope on stdout.\nErrors and verbose diagnostics use stderr. --output jsonl and table are also available."
	if effect == "write" {
		root.Long += "\nAdvertising writes require explicit --profile and --account, plus a write-mode profile.\nCommands exposing --dry-run do not modify advertising in that mode. Unknown create outcomes exit 10 and require reconciliation."
	}
	root.Example = "  " + root.CommandPath() + " --help\n  adsctl commands --platform meta --capability " + effect
}

package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"adsctl/internal/output"
	metaschema "adsctl/internal/platform/meta/schema"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func (a *App) addDiscovery(root, meta *cobra.Command) {
	var platform, capability string
	commands := action("commands", "Discover implemented command contracts", "local", func(c *cobra.Command, _ []string) error { return a.emit(c, commandRows(root, platform, capability)) })
	commands.Flags().StringVar(&platform, "platform", "", "Filter by platform")
	commands.Flags().StringVar(&capability, "capability", "", "Filter effect: read, write, report-job, local")
	root.AddCommand(commands)
	var format string
	docs := action("docs", "Generate agent documentation from implemented commands", "local", func(_ *cobra.Command, _ []string) error {
		if format != "markdown" {
			return output.Fail("USAGE", "Docs format must be markdown.", 2)
		}
		if _, err := io.WriteString(a.Out, skillsHint); err != nil {
			return err
		}
		for _, row := range commandRows(root, "", "") {
			m := row.(map[string]any)
			if _, err := fmt.Fprintf(a.Out, "## `%s`\n\n%s\n\nEffect: %s\n\n", m["usage"], m["description"], m["effect"]); err != nil {
				return err
			}
		}
		return nil
	})
	docs.Flags().StringVar(&format, "format", "markdown", "Documentation format")
	root.AddCommand(docs)
	schema := group("schema", "Inspect supported command schemas")
	meta.AddCommand(schema)
	var operation string
	get := action("get RESOURCE", "Describe supported flags and operations", "local", func(c *cobra.Command, args []string) error {
		var rows []any
		for _, row := range commandRows(meta, "", "") {
			m := row.(map[string]any)
			parts := strings.Fields(m["path"].(string))
			if len(parts) >= 3 && parts[2] == args[0] && (operation == "" || (len(parts) >= 4 && parts[3] == operation)) {
				rows = append(rows, row)
			}
		}
		if len(rows) == 0 {
			return output.Fail("SCHEMA_UNSUPPORTED", "Resource or operation is not implemented.", 2)
		}
		version := a.opts.APIVersion
		if version == "" {
			version = "v25.0"
		}
		return a.emit(c, map[string]any{"resource": args[0], "operation": operation, "commands": rows, "coverage": "implemented CLI subset; provider validates conditional fields", "api_version": version, "writable_fields": metaschema.Fields(args[0], operation), "nested_fields": "native JSON; provider validates conditional requirements"})
	})
	get.Args = cobra.ExactArgs(1)
	get.Flags().StringVar(&operation, "operation", "", "Operation name")
	schema.AddCommand(get)
}
func commandRows(root *cobra.Command, platform, capability string) []any {
	rows := []any{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.HasSubCommands() {
			for _, child := range c.Commands() {
				walk(child)
			}
			return
		}
		if c.Hidden || c.RunE == nil {
			return
		}
		effect := c.Annotations["effect"]
		if effect == "" {
			effect = "local"
		}
		if capability != "" && effect != capability {
			return
		}
		if platform != "" && !strings.Contains(c.CommandPath(), " "+platform+" ") {
			return
		}
		flags := []any{}
		add := func(f *pflag.Flag) {
			flags = append(flags, map[string]any{"name": f.Name, "type": f.Value.Type(), "default": f.DefValue, "description": f.Usage})
		}
		c.LocalNonPersistentFlags().VisitAll(add)
		c.InheritedFlags().VisitAll(add)
		rows = append(rows, map[string]any{"path": c.CommandPath(), "usage": c.UseLine(), "description": c.Short, "effect": effect, "flags": flags, "schema_version": "1", "output_formats": []string{"json", "jsonl", "table"}})
	}
	walk(root)
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].(map[string]any)["path"].(string) < rows[j].(map[string]any)["path"].(string)
	})
	return rows
}

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var errDocsUsage = errors.New("docs format must be one of: markdown, man")

func (a *App) runDocs(args []string) error {
	out := a.Out
	if out == nil {
		out = io.Discard
	}
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(out, "Usage: "+a.Name+" docs [markdown|man] [output_dir]")
		return errDocsUsage
	}

	spec := a.Spec()
	switch args[0] {
	case "markdown":
		if len(args) == 2 {
			return writeMarkdownBundle(spec, args[1])
		}
		_, err := io.WriteString(out, markdownDocs(spec))
		return err
	case "man":
		if len(args) == 2 {
			return writeManBundle(spec, args[1])
		}
		_, err := io.WriteString(out, manDocs(spec))
		return err
	default:
		fmt.Fprintln(out, "Usage: "+a.Name+" docs [markdown|man] [output_dir]")
		return errDocsUsage
	}
}

func markdownDocs(spec AppSpec) string {
	var b strings.Builder
	writeMarkdownFrontMatter(&b, appFrontMatter(spec))
	fmt.Fprintf(&b, "# %s\n\n", spec.Name)
	if spec.Short != "" {
		fmt.Fprintf(&b, "%s\n\n", spec.Short)
	}

	fmt.Fprintf(&b, "## Usage\n\n")
	if spec.Root != nil && spec.Root.UsageLine != "" {
		fmt.Fprintf(&b, "`%s`\n\n", spec.Root.UsageLine)
	} else {
		fmt.Fprintf(&b, "`%s [flags] <command> [subcommand] [args]`\n\n", spec.Name)
	}
	if spec.Root != nil && spec.Root.Long != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(spec.Root.Long))
	}

	if len(spec.Builtins) > 0 {
		fmt.Fprintf(&b, "## Built-ins\n\n")
		for _, builtin := range spec.Builtins {
			fmt.Fprintf(&b, "- `%s`\n", builtin)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(spec.GlobalFlags) > 0 {
		writeMarkdownFlags(&b, "## Global Flags", spec.GlobalFlags)
	}
	if spec.Root != nil && len(spec.Root.Positionals) > 0 {
		writeMarkdownPositionals(&b, "## Positional Arguments", spec.Root.Positionals)
	}
	if spec.Root != nil && len(spec.Root.Examples) > 0 {
		writeMarkdownExamples(&b, "## Examples", spec.Root.Examples)
	}

	if len(spec.Commands) > 0 {
		fmt.Fprintf(&b, "## Commands\n\n")
		for _, cmd := range spec.Commands {
			writeMarkdownCommand(&b, spec.Name, cmd, 3, nil)
		}
	}

	return b.String()
}

func writeMarkdownCommand(b *strings.Builder, parent string, cmd CommandSpec, level int, subcommandPath func(CommandSpec) string) {
	header := strings.Repeat("#", level)
	fullName := strings.TrimSpace(parent + " " + cmd.Name)
	if level == 1 {
		writeMarkdownFrontMatter(b, commandFrontMatter(fullName, cmd))
	}
	fmt.Fprintf(b, "%s `%s`\n\n", header, fullName)

	if cmd.Short != "" {
		fmt.Fprintf(b, "%s\n\n", cmd.Short)
	}
	if cmd.UsageLine != "" {
		fmt.Fprintf(b, "**Usage:** `%s`\n\n", cmd.UsageLine)
	}
	if cmd.Deprecated != "" {
		fmt.Fprintf(b, "**Deprecated:** %s\n\n", cmd.Deprecated)
	}
	if len(cmd.Aliases) > 0 {
		fmt.Fprintf(b, "**Aliases:** `%s`\n\n", strings.Join(cmd.Aliases, "`, `"))
	}
	if cmd.Long != "" {
		fmt.Fprintf(b, "%s\n\n", strings.TrimSpace(cmd.Long))
	}
	if len(cmd.Flags) > 0 {
		writeMarkdownFlags(b, "#### Flags", cmd.Flags)
	}
	if len(cmd.Positionals) > 0 {
		writeMarkdownPositionals(b, "#### Positional Arguments", cmd.Positionals)
	}
	if len(cmd.Examples) > 0 {
		writeMarkdownExamples(b, "#### Examples", cmd.Examples)
	}
	if len(cmd.SubCommands) > 0 && subcommandPath != nil {
		fmt.Fprintf(b, "#### Subcommands\n\n")
		for _, sub := range cmd.SubCommands {
			fmt.Fprintf(b, "- [`%s`](%s)\n", sub.Name, subcommandPath(sub))
		}
		fmt.Fprintln(b)
	}
	for _, sub := range cmd.SubCommands {
		writeMarkdownCommand(b, fullName, sub, level+1, subcommandPath)
	}
}

func writeMarkdownFlags(b *strings.Builder, title string, flags []FlagSpec) {
	fmt.Fprintf(b, "%s\n\n", title)
	for _, flag := range flags {
		fmt.Fprintf(b, "- `--%s`", flag.Name)
		if flag.Shorthand != "" {
			fmt.Fprintf(b, " / `-%s`", flag.Shorthand)
		}
		if flag.Type != "" {
			fmt.Fprintf(b, " <%s>", flag.Type)
		}
		if flag.Usage != "" {
			fmt.Fprintf(b, " %s", flag.Usage)
		}
		notes := make([]string, 0)
		if flag.Required {
			notes = append(notes, "required")
		}
		if flag.Default != "" {
			notes = append(notes, "default: "+flag.Default)
		}
		if len(flag.Enum) > 0 {
			notes = append(notes, "choices: "+strings.Join(flag.Enum, ", "))
		}
		if len(flag.EnvVars) > 0 {
			notes = append(notes, "env: "+strings.Join(flag.EnvVars, ", "))
		}
		if len(flag.ConfigKeys) > 0 {
			notes = append(notes, "config: "+strings.Join(flag.ConfigKeys, ", "))
		}
		if len(notes) > 0 {
			fmt.Fprintf(b, " (%s)", strings.Join(notes, "; "))
		}
		fmt.Fprintln(b)
	}
	fmt.Fprintln(b)
}

func writeMarkdownPositionals(b *strings.Builder, title string, positionals []PositionalSpec) {
	fmt.Fprintf(b, "%s\n\n", title)
	for _, positional := range positionals {
		fmt.Fprintf(b, "- `%s`", positional.Name)
		if positional.Required {
			fmt.Fprintf(b, " required")
		}
		if positional.Variadic {
			fmt.Fprintf(b, " variadic")
		}
		if positional.Usage != "" {
			fmt.Fprintf(b, " %s", positional.Usage)
		}
		if len(positional.Enum) > 0 {
			fmt.Fprintf(b, " choices: `%s`", strings.Join(positional.Enum, "`, `"))
		}
		fmt.Fprintln(b)
	}
	fmt.Fprintln(b)
}

func writeMarkdownExamples(b *strings.Builder, title string, examples []string) {
	fmt.Fprintf(b, "%s\n\n", title)
	for _, example := range examples {
		fmt.Fprintf(b, "- `%s`\n", example)
	}
	fmt.Fprintln(b)
}

func manDocs(spec AppSpec) string {
	var b strings.Builder
	date := time.Now().Format("2006-01-02")
	fmt.Fprintf(&b, ".TH %s 1 %s\n", strings.ToUpper(spec.Name), date)
	fmt.Fprintf(&b, ".SH NAME\n%s \\- %s\n", spec.Name, escapeMan(spec.Short))
	if spec.Root != nil && spec.Root.UsageLine != "" {
		fmt.Fprintf(&b, ".SH SYNOPSIS\n\\fB%s\\fR\n", escapeMan(spec.Root.UsageLine))
	} else {
		fmt.Fprintf(&b, ".SH SYNOPSIS\n\\fB%s\\fR [flags] <command> [subcommand] [args]\n", spec.Name)
	}
	description := spec.Short
	if spec.Root != nil && spec.Root.Long != "" {
		description = spec.Root.Long
	}
	fmt.Fprintf(&b, ".SH DESCRIPTION\n%s\n", escapeMan(description))

	if len(spec.GlobalFlags) > 0 {
		fmt.Fprintf(&b, ".SH GLOBAL FLAGS\n")
		writeManFlags(&b, spec.GlobalFlags)
	}
	if spec.Root != nil && len(spec.Root.Positionals) > 0 {
		fmt.Fprintf(&b, ".SH POSITIONAL ARGUMENTS\n")
		writeManPositionals(&b, spec.Root.Positionals)
	}
	if spec.Root != nil && len(spec.Root.Examples) > 0 {
		fmt.Fprintf(&b, ".SH EXAMPLES\n")
		writeManExamples(&b, spec.Root.Examples)
	}

	if len(spec.Commands) > 0 {
		fmt.Fprintf(&b, ".SH COMMANDS\n")
		for _, cmd := range spec.Commands {
			writeManCommand(&b, spec.Name, cmd)
		}
	}

	return b.String()
}

func writeManCommand(b *strings.Builder, parent string, cmd CommandSpec) {
	fullName := strings.TrimSpace(parent + " " + cmd.Name)
	fmt.Fprintf(b, ".SS %s\n", escapeMan(fullName))
	if cmd.Short != "" {
		fmt.Fprintf(b, "%s\n", escapeMan(cmd.Short))
	}
	if cmd.UsageLine != "" {
		fmt.Fprintf(b, ".PP\nUsage: \\fB%s\\fR\n", escapeMan(cmd.UsageLine))
	}
	if len(cmd.Flags) > 0 {
		fmt.Fprintf(b, ".PP\nFlags:\n")
		writeManFlags(b, cmd.Flags)
	}
	if len(cmd.Positionals) > 0 {
		fmt.Fprintf(b, ".PP\nPositional arguments:\n")
		writeManPositionals(b, cmd.Positionals)
	}
	if len(cmd.Examples) > 0 {
		fmt.Fprintf(b, ".PP\nExamples:\n")
		writeManExamples(b, cmd.Examples)
	}
	for _, sub := range cmd.SubCommands {
		writeManCommand(b, fullName, sub)
	}
}

func writeManPositionals(b *strings.Builder, positionals []PositionalSpec) {
	for _, positional := range positionals {
		fmt.Fprintf(b, ".IP \\fB%s\\fR 4\n", escapeMan(positional.Name))
		line := positional.Usage
		if line == "" {
			line = "argument"
		}
		if len(positional.Enum) > 0 {
			line += " (choices: " + strings.Join(positional.Enum, ", ") + ")"
		}
		fmt.Fprintf(b, "%s\n", escapeMan(line))
	}
}

func writeManExamples(b *strings.Builder, examples []string) {
	for _, example := range examples {
		fmt.Fprintf(b, ".IP \\[bu] 2\n\\fB%s\\fR\n", escapeMan(example))
	}
}

func writeManFlags(b *strings.Builder, flags []FlagSpec) {
	for _, flag := range flags {
		fmt.Fprintf(b, ".TP\n")
		if flag.Shorthand != "" {
			fmt.Fprintf(b, "\\fB-%s\\fR, ", escapeMan(flag.Shorthand))
		}
		fmt.Fprintf(b, "\\fB--%s\\fR", escapeMan(flag.Name))
		if flag.Type != "" {
			fmt.Fprintf(b, " <%s>", escapeMan(flag.Type))
		}
		line := flag.Usage
		notes := make([]string, 0)
		if flag.Required {
			notes = append(notes, "required")
		}
		if flag.Default != "" {
			notes = append(notes, "default: "+flag.Default)
		}
		if len(flag.Enum) > 0 {
			notes = append(notes, "choices: "+strings.Join(flag.Enum, ", "))
		}
		if len(notes) > 0 {
			if line != "" {
				line += " "
			}
			line += "(" + strings.Join(notes, "; ") + ")"
		}
		fmt.Fprintf(b, "%s\n", escapeMan(line))
	}
}

func escapeMan(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "-", `\-`)
	return replacer.Replace(s)
}

func writeMarkdownBundle(spec AppSpec, outputDir string) error {
	if err := os.MkdirAll(filepath.Join(outputDir, "commands"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "README.md"), []byte(markdownBundleIndex(spec)), 0o644); err != nil {
		return err
	}
	for _, cmd := range spec.Commands {
		if err := writeMarkdownCommandFiles(spec.Name, outputDir, nil, cmd); err != nil {
			return err
		}
	}
	return nil
}

func markdownBundleIndex(spec AppSpec) string {
	var b strings.Builder
	writeMarkdownFrontMatter(&b, appFrontMatter(spec))
	fmt.Fprintf(&b, "# %s\n\n", spec.Name)
	if spec.Short != "" {
		fmt.Fprintf(&b, "%s\n\n", spec.Short)
	}
	fmt.Fprintf(&b, "## Command Pages\n\n")
	for _, cmd := range spec.Commands {
		writeMarkdownCommandLinks(&b, nil, cmd)
	}
	fmt.Fprintln(&b)
	return b.String()
}

func writeMarkdownCommandLinks(b *strings.Builder, parents []string, cmd CommandSpec) {
	segments := append(append([]string(nil), parents...), cmd.Name)
	fmt.Fprintf(b, "- [`%s`](commands/%s.md)\n", strings.Join(segments, " "), strings.Join(segments, "/"))
	for _, sub := range cmd.SubCommands {
		writeMarkdownCommandLinks(b, segments, sub)
	}
}

func writeMarkdownCommandFiles(appName string, outputDir string, parents []string, cmd CommandSpec) error {
	segments := append(append([]string(nil), parents...), cmd.Name)
	target := filepath.Join(outputDir, "commands", filepath.Join(segments...)+".md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	currentDir := filepath.Dir(target)

	var b strings.Builder
	writeMarkdownCommand(&b, joinCommandPath(appName, parents), cmd, 1, func(sub CommandSpec) string {
		subSegments := append(append([]string(nil), segments...), sub.Name)
		subTarget := filepath.Join(outputDir, "commands", filepath.Join(subSegments...)+".md")
		relative, err := filepath.Rel(currentDir, subTarget)
		if err != nil {
			return filepath.ToSlash(filepath.Join(subSegments...) + ".md")
		}
		return filepath.ToSlash(relative)
	})
	if err := os.WriteFile(target, []byte(b.String()), 0o644); err != nil {
		return err
	}

	for _, sub := range cmd.SubCommands {
		if err := writeMarkdownCommandFiles(appName, outputDir, segments, sub); err != nil {
			return err
		}
	}
	return nil
}

func writeManBundle(spec AppSpec, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, spec.Name+".1"), []byte(manDocs(spec)), 0o644); err != nil {
		return err
	}
	for _, cmd := range spec.Commands {
		if err := writeManCommandFiles(spec.Name, outputDir, nil, cmd); err != nil {
			return err
		}
	}
	return nil
}

func writeManCommandFiles(appName string, outputDir string, parents []string, cmd CommandSpec) error {
	segments := append(append([]string(nil), parents...), cmd.Name)
	filename := appName + "-" + strings.Join(segments, "-") + ".1"

	var b strings.Builder
	date := time.Now().Format("2006-01-02")
	fmt.Fprintf(&b, ".TH %s 1 %s\n", strings.ToUpper(appName+"-"+strings.Join(segments, "-")), date)
	writeManCommand(&b, joinCommandPath(appName, parents), cmd)

	if err := os.WriteFile(filepath.Join(outputDir, filename), []byte(b.String()), 0o644); err != nil {
		return err
	}
	for _, sub := range cmd.SubCommands {
		if err := writeManCommandFiles(appName, outputDir, segments, sub); err != nil {
			return err
		}
	}
	return nil
}

func joinCommandPath(head string, tail []string) string {
	parts := []string{head}
	parts = append(parts, tail...)
	return strings.Join(parts, " ")
}

func writeMarkdownFrontMatter(b *strings.Builder, data map[string]any) {
	if len(data) == 0 {
		return
	}
	b.WriteString("---\n")
	writeYAMLMap(b, data, 0)
	b.WriteString("---\n\n")
}

func appFrontMatter(spec AppSpec) map[string]any {
	data := map[string]any{
		"title":          spec.Name,
		"kind":           "app",
		"app":            spec.Name,
		"schema_version": spec.SchemaVersion,
		"builtins":       spec.Builtins,
	}
	if spec.Short != "" {
		data["summary"] = spec.Short
	}
	if len(spec.Extensions) > 0 {
		data["extensions"] = cloneExtensions(spec.Extensions)
	}
	return data
}

func commandFrontMatter(fullName string, cmd CommandSpec) map[string]any {
	data := map[string]any{
		"title":           fullName,
		"kind":            "command",
		"command_name":    cmd.Name,
		"command_path":    strings.Split(fullName, " "),
		"runnable":        cmd.Runnable,
		"hidden":          cmd.Hidden,
		"has_subcommands": len(cmd.SubCommands) > 0,
	}
	if cmd.Short != "" {
		data["summary"] = cmd.Short
	}
	if len(cmd.Aliases) > 0 {
		data["aliases"] = cmd.Aliases
	}
	if len(cmd.Extensions) > 0 {
		data["extensions"] = cloneExtensions(cmd.Extensions)
	}
	return data
}

func writeYAMLMap(b *strings.Builder, data map[string]any, indent int) {
	keys := sortedStringKeys(data)
	for _, key := range keys {
		writeYAMLValue(b, key, data[key], indent)
	}
}

func writeYAMLValue(b *strings.Builder, key string, value any, indent int) {
	padding := strings.Repeat("  ", indent)
	switch typed := value.(type) {
	case nil:
		fmt.Fprintf(b, "%s%s: null\n", padding, key)
	case string:
		fmt.Fprintf(b, "%s%s: %q\n", padding, key, typed)
	case bool:
		fmt.Fprintf(b, "%s%s: %t\n", padding, key, typed)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		fmt.Fprintf(b, "%s%s: %v\n", padding, key, typed)
	case []string:
		if len(typed) == 0 {
			fmt.Fprintf(b, "%s%s: []\n", padding, key)
			return
		}
		fmt.Fprintf(b, "%s%s:\n", padding, key)
		for _, item := range typed {
			fmt.Fprintf(b, "%s  - %q\n", padding, item)
		}
	case []any:
		if len(typed) == 0 {
			fmt.Fprintf(b, "%s%s: []\n", padding, key)
			return
		}
		fmt.Fprintf(b, "%s%s:\n", padding, key)
		for _, item := range typed {
			switch nested := item.(type) {
			case map[string]any:
				fmt.Fprintf(b, "%s  -\n", padding)
				writeYAMLMap(b, nested, indent+2)
			case string:
				fmt.Fprintf(b, "%s  - %q\n", padding, nested)
			default:
				fmt.Fprintf(b, "%s  - %v\n", padding, nested)
			}
		}
	case map[string]any:
		if len(typed) == 0 {
			fmt.Fprintf(b, "%s%s: {}\n", padding, key)
			return
		}
		fmt.Fprintf(b, "%s%s:\n", padding, key)
		writeYAMLMap(b, typed, indent+1)
	default:
		fmt.Fprintf(b, "%s%s: %q\n", padding, key, fmt.Sprint(value))
	}
}

func sortedStringKeys(data map[string]any) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

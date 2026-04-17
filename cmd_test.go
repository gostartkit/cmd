package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAppRun(t *testing.T) {
	var runCount int
	var capturedArgs []string

	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "foo",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				runCount++
				capturedArgs = args
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"foo", "arg1", "arg2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runCount != 1 {
		t.Errorf("expected runCount 1, got %d", runCount)
	}

	if len(capturedArgs) != 2 || capturedArgs[0] != "arg1" || capturedArgs[1] != "arg2" {
		t.Errorf("unexpected args: %v", capturedArgs)
	}
}

func TestAppSubCommand(t *testing.T) {
	var subRunCount int

	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "foo",
			SubCommands: []*Command{
				{
					Name: "bar",
					Run: func(ctx context.Context, cmd *Command, args []string) error {
						subRunCount++
						return nil
					},
				},
			},
		},
	}

	err := app.Run(context.Background(), []string{"foo", "bar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if subRunCount != 1 {
		t.Errorf("expected subRunCount 1, got %d", subRunCount)
	}
}

func TestAppFlags(t *testing.T) {
	app := NewApp("test")
	var verbose bool
	var count int

	app.Commands = []*Command{
		{
			Name: "foo",
			SetFlags: func(f *FlagSet) {
				f.BoolVar(&verbose, "verbose", false, "v", "v")
				f.IntVar(&count, "count", 0, "c", "c")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"foo", "-v", "--count=10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !verbose {
		t.Errorf("expected verbose true")
	}

	if count != 10 {
		t.Errorf("expected count 10, got %d", count)
	}
}

func TestAppFlagsAfterPositionalArgs(t *testing.T) {
	app := NewApp("test")
	var verbose bool
	var name string
	var capturedArgs []string

	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				f.BoolVar(&verbose, "verbose", false, "v", "v")
				f.StringVar(&name, "name", "", "n", "n")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				capturedArgs = args
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"sayhi", "abc", "-v", "-n", "sam", "xyz"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !verbose {
		t.Errorf("expected verbose true")
	}

	if name != "sam" {
		t.Errorf("expected name sam, got %q", name)
	}

	if len(capturedArgs) != 2 || capturedArgs[0] != "abc" || capturedArgs[1] != "xyz" {
		t.Errorf("unexpected args: %v", capturedArgs)
	}
}

func TestAppFlagsStopAtDoubleDash(t *testing.T) {
	app := NewApp("test")
	var verbose bool
	var capturedArgs []string

	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				f.BoolVar(&verbose, "verbose", false, "v", "v")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				capturedArgs = args
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"sayhi", "abc", "--", "-v"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verbose {
		t.Errorf("expected verbose false")
	}

	if len(capturedArgs) != 2 || capturedArgs[0] != "abc" || capturedArgs[1] != "-v" {
		t.Errorf("unexpected args: %v", capturedArgs)
	}
}

func TestAppGlobalFlagsBeforeCommand(t *testing.T) {
	app := NewApp("test")
	var verbose bool
	var name string

	app.SetFlags = func(f *FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
	}
	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				f.StringVar(&name, "name", "", "name value", "n")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"--verbose", "sayhi", "--name", "sam"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !verbose {
		t.Fatalf("expected verbose true")
	}

	if name != "sam" {
		t.Fatalf("expected name sam, got %q", name)
	}
}

func TestAppHelpCommandShowsFlags(t *testing.T) {
	app := NewApp("test")
	var output bytes.Buffer
	app.Out = &output
	app.Err = &output

	app.Commands = []*Command{
		{
			Name:      "sayhi",
			UsageLine: "test sayhi",
			SetFlags: func(f *FlagSet) {
				var name string
				f.StringVar(&name, "name", "", "name value", "n")
				f.SetExample("name", "sam")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"help", "sayhi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	helpOutput := output.String()
	if !strings.Contains(helpOutput, "--name <string>") {
		t.Fatalf("expected help output to include command flags, got %q", helpOutput)
	}

	if !strings.Contains(helpOutput, "example: sam") {
		t.Fatalf("expected help output to include flag metadata, got %q", helpOutput)
	}
}

func TestAppHelpCommandAfterGlobalFlags(t *testing.T) {
	app := NewApp("test")
	var output bytes.Buffer
	app.Out = &output
	app.Err = &output
	app.SetFlags = func(f *FlagSet) {
		var verbose bool
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
	}
	app.Commands = []*Command{
		{
			Name:      "sayhi",
			UsageLine: "test sayhi",
			SetFlags: func(f *FlagSet) {
				var name string
				f.StringVar(&name, "name", "", "name value", "n")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"--verbose", "help", "sayhi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output.String(), "--name <string>") {
		t.Fatalf("expected help output after global flags, got %q", output.String())
	}
}

func TestAppHelpShowsPositionals(t *testing.T) {
	app := NewApp("test")
	var output bytes.Buffer
	app.Out = &output
	app.Err = &output
	app.Commands = []*Command{
		{
			Name:      "deploy",
			UsageLine: "test deploy <env>",
			Positionals: []PositionalArg{
				{Name: "env", Usage: "target environment", Required: true, Enum: []string{"dev", "prod"}, Example: "dev"},
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"help", "deploy"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	helpOutput := output.String()
	if !strings.Contains(helpOutput, "Arguments:") || !strings.Contains(helpOutput, "<env>") {
		t.Fatalf("expected positional arguments in help, got %q", helpOutput)
	}
	if !strings.Contains(helpOutput, "choices: dev, prod") {
		t.Fatalf("expected positional choices in help, got %q", helpOutput)
	}
}

func TestAppCompleteRootSuggestions(t *testing.T) {
	app := NewApp("test")
	app.SetFlags = func(f *FlagSet) {
		var verbose bool
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
	}
	app.Commands = []*Command{
		{Name: "sayhi"},
		{Name: "status"},
	}

	got := app.complete([]string{""})
	want := []string{"--verbose", "-v", "sayhi", "status", "help", "completion", "spec", "docs"}
	for _, expected := range want {
		if !slices.Contains(got, expected) {
			t.Fatalf("expected %q in suggestions, got %v", expected, got)
		}
	}
}

func TestAppCompleteBuiltinArguments(t *testing.T) {
	app := NewApp("test")

	got := app.complete([]string{"docs", ""})
	if !slices.Equal(got, []string{"man", "markdown"}) {
		t.Fatalf("expected docs builtin suggestions, got %v", got)
	}

	got = app.complete([]string{"completion", ""})
	if !slices.Equal(got, []string{"bash", "fish", "powershell", "zsh"}) {
		t.Fatalf("expected completion builtin suggestions, got %v", got)
	}

	got = app.complete([]string{"spec", ""})
	if !slices.Equal(got, []string{"json"}) {
		t.Fatalf("expected spec builtin suggestions, got %v", got)
	}
}

func TestAppCompleteCommandFlags(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				var name string
				f.StringVar(&name, "name", "", "name value", "n")
				f.SetEnum("name", "sam", "sara")
			},
		},
	}

	got := app.complete([]string{"sayhi", "--n"})
	if len(got) != 1 || got[0] != "--name" {
		t.Fatalf("expected --name suggestion, got %v", got)
	}
}

func TestAppCompleteFlagEnumValues(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				var format string
				f.StringVar(&format, "format", "", "output format", "f")
				f.SetEnum("format", "json", "yaml", "text")
			},
		},
	}

	got := app.complete([]string{"sayhi", "--format", "j"})
	if len(got) != 1 || got[0] != "json" {
		t.Fatalf("expected json enum suggestion, got %v", got)
	}

	got = app.complete([]string{"sayhi", "--format=j"})
	if len(got) != 1 || got[0] != "--format=json" {
		t.Fatalf("expected attached enum suggestion, got %v", got)
	}
}

func TestAppCompleteFlagDynamicValues(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				var name string
				f.StringVar(&name, "name", "", "name value", "n")
				f.SetCompletion("name", func(ctx CompletionContext) []string {
					return []string{"sam", "sara", "tom"}
				})
			},
		},
	}

	got := app.complete([]string{"sayhi", "--name", "sa"})
	if len(got) != 2 || got[0] != "sam" || got[1] != "sara" {
		t.Fatalf("expected dynamic suggestions, got %v", got)
	}
}

func TestAppCompleteAfterGlobalFlags(t *testing.T) {
	app := NewApp("test")
	app.SetFlags = func(f *FlagSet) {
		var verbose bool
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
	}
	app.Commands = []*Command{
		{Name: "sayhi"},
	}

	got := app.complete([]string{"--verbose", ""})
	if !slices.Contains(got, "sayhi") {
		t.Fatalf("expected sayhi suggestion after global flag, got %v", got)
	}
}

func TestAppCompletePositionalEnumValues(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "deploy",
			Positionals: []PositionalArg{
				{Name: "env", Required: true, Enum: []string{"dev", "prod"}},
			},
		},
	}

	got := app.complete([]string{"deploy", "d"})
	if len(got) != 1 || got[0] != "dev" {
		t.Fatalf("expected positional enum completion, got %v", got)
	}
}

func TestAppCompletePositionalDynamicValues(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "deploy",
			Positionals: []PositionalArg{
				{
					Name: "env",
					Completion: func(ctx CompletionContext) []string {
						return []string{"dev", "demo", "prod"}
					},
				},
			},
		},
	}

	got := app.complete([]string{"deploy", "de"})
	if len(got) != 2 || got[0] != "demo" || got[1] != "dev" {
		t.Fatalf("expected positional dynamic completion, got %v", got)
	}
}

func TestAppCompletionCommandOutputsScript(t *testing.T) {
	app := NewApp("test")
	var output bytes.Buffer
	app.Out = &output
	app.Err = &output

	err := app.Run(context.Background(), []string{"completion", "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output.String(), "__complete") {
		t.Fatalf("expected completion script output, got %q", output.String())
	}
}

func TestAppSpecCommandOutputsJSON(t *testing.T) {
	app := NewApp("test")
	app.EnableConfigSupport()
	app.BeforeRun = func(ctx HookContext) error { return nil }
	app.Use(func(ctx MiddlewareContext, next NextFunc) error {
		return next(ctx.Context)
	})
	app.AddObserver(ObserverFunc(func(event Event) {}))
	app.SetExtension("x-site-section", "cli")
	var output bytes.Buffer
	app.Out = &output
	app.Err = &output
	app.SetFlags = func(f *FlagSet) {
		var verbose bool
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
		f.SetCategory("verbose", "Global")
	}
	app.Commands = []*Command{
		{
			Name:      "sayhi",
			UsageLine: "test sayhi",
			Examples:  []string{"test sayhi --name sam"},
			Positionals: []PositionalArg{
				{
					Name:       "target",
					Usage:      "target subject",
					Required:   true,
					Enum:       []string{"team", "user"},
					Extensions: map[string]any{"x-label": "Target"},
				},
				{
					Name: "service",
					Completion: func(ctx CompletionContext) []string {
						return []string{"api", "web"}
					},
				},
			},
			BeforeRun: func(ctx HookContext) error { return nil },
			OnError:   func(ctx HookContext) {},
			Middlewares: []Middleware{
				func(ctx MiddlewareContext, next NextFunc) error {
					return next(ctx.Context)
				},
			},
			Observers: []Observer{
				ObserverFunc(func(event Event) {}),
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
			Extensions: map[string]any{"x-owner": "platform"},
			SetFlags: func(f *FlagSet) {
				var name string
				f.StringVar(&name, "name", "", "name value", "n")
				f.BindEnv("name", "APP_NAME")
				f.BindConfig("name", "name")
				f.SetEnum("name", "sam", "sara")
				f.MarkRequired("name")
				if err := f.SetExtension("name", "x-ui-control", "user-picker"); err != nil {
					t.Fatalf("set flag extension: %v", err)
				}
			},
		},
	}

	err := app.Run(context.Background(), []string{"spec"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var spec AppSpec
	if err := json.Unmarshal(output.Bytes(), &spec); err != nil {
		t.Fatalf("expected valid json, got %v", err)
	}

	if spec.Name != "test" {
		t.Fatalf("expected app name test, got %q", spec.Name)
	}

	if spec.SchemaVersion != "v2" {
		t.Fatalf("expected schema version v2, got %q", spec.SchemaVersion)
	}

	if !spec.Capabilities.Middleware || !spec.Capabilities.Observers || !spec.Capabilities.LifecycleHooks || !spec.Capabilities.DocsExport {
		t.Fatalf("expected modern capabilities in spec, got %+v", spec.Capabilities)
	}

	if !spec.Config.Enabled || spec.Config.FlagName != "config" {
		t.Fatalf("expected config runtime details in spec, got %+v", spec.Config)
	}

	if !spec.Hooks.BeforeRun || spec.Hooks.AfterRun || spec.Hooks.OnError {
		t.Fatalf("unexpected app hook spec: %+v", spec.Hooks)
	}

	if !spec.HasMiddleware || !spec.HasObservers {
		t.Fatalf("expected app middleware/observer metadata, got middleware=%v observers=%v", spec.HasMiddleware, spec.HasObservers)
	}
	if spec.Extensions["x-site-section"] != "cli" {
		t.Fatalf("expected app extensions in spec, got %+v", spec.Extensions)
	}

	if !slices.Equal(spec.Builtins, []string{"help", "completion", "spec", "docs"}) {
		t.Fatalf("unexpected builtins in spec: %+v", spec.Builtins)
	}

	if !slices.ContainsFunc(spec.GlobalFlags, func(flag FlagSpec) bool { return flag.Name == "verbose" }) {
		t.Fatalf("expected global flags in spec, got %+v", spec.GlobalFlags)
	}

	if !slices.ContainsFunc(spec.GlobalFlags, func(flag FlagSpec) bool { return flag.Name == "config" }) {
		t.Fatalf("expected builtin config flag in spec, got %+v", spec.GlobalFlags)
	}

	if len(spec.Commands) != 1 || spec.Commands[0].Name != "sayhi" {
		t.Fatalf("expected sayhi command in spec, got %+v", spec.Commands)
	}

	if !spec.Commands[0].Runnable || !spec.Commands[0].Hooks.BeforeRun || spec.Commands[0].Hooks.AfterRun || !spec.Commands[0].Hooks.OnError {
		t.Fatalf("expected command hook metadata, got %+v", spec.Commands[0])
	}

	if !spec.Commands[0].HasMiddleware || !spec.Commands[0].HasObservers {
		t.Fatalf("expected command middleware/observer metadata, got %+v", spec.Commands[0])
	}
	if spec.Commands[0].Extensions["x-owner"] != "platform" {
		t.Fatalf("expected command extensions in spec, got %+v", spec.Commands[0].Extensions)
	}

	if !slices.ContainsFunc(spec.Commands[0].Flags, func(flag FlagSpec) bool {
		return flag.Name == "name" &&
			flag.Required &&
			len(flag.ConfigKeys) == 1 &&
			flag.ConfigKeys[0] == "name" &&
			len(flag.Enum) == 2 &&
			flag.SupportsCompletion &&
			slices.Equal(flag.SourceOrder, []string{"cli", "env", "config", "default"}) &&
			flag.Extensions["x-ui-control"] == "user-picker"
	}) {
		t.Fatalf("expected required name flag in spec, got %+v", spec.Commands[0].Flags)
	}

	if len(spec.Commands[0].Positionals) != 2 || spec.Commands[0].Positionals[0].Name != "target" || len(spec.Commands[0].Positionals[0].Enum) != 2 {
		t.Fatalf("expected positional spec, got %+v", spec.Commands[0].Positionals)
	}

	if !spec.Commands[0].Positionals[0].SupportsCompletion || !spec.Commands[0].Positionals[1].SupportsCompletion {
		t.Fatalf("expected positional completion metadata, got %+v", spec.Commands[0].Positionals)
	}
	if spec.Commands[0].Positionals[0].Extensions["x-label"] != "Target" {
		t.Fatalf("expected positional extensions in spec, got %+v", spec.Commands[0].Positionals)
	}
}

func TestAppDocsMarkdownOutput(t *testing.T) {
	app := NewApp("test")
	app.SetExtension("x-site-section", "cli")
	var output bytes.Buffer
	app.Out = &output
	app.Err = &output
	app.SetFlags = func(f *FlagSet) {
		var verbose bool
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
	}
	app.Commands = []*Command{
		{
			Name:       "deploy",
			UsageLine:  "test deploy <env>",
			Short:      "deploy service",
			Examples:   []string{"test deploy prod --force"},
			Extensions: map[string]any{"x-owner": "platform"},
			Positionals: []PositionalArg{
				{Name: "env", Usage: "target environment", Required: true, Enum: []string{"dev", "prod"}},
			},
			SetFlags: func(f *FlagSet) {
				var force bool
				f.BoolVar(&force, "force", false, "force deploy", "f")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	if err := app.Run(context.Background(), []string{"docs", "markdown"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := output.String()
	for _, want := range []string{
		"---",
		`kind: "app"`,
		`extensions:`,
		`x-site-section: "cli"`,
		"# test",
		"## Built-ins",
		"## Commands",
		"### `test deploy`",
		"**Usage:** `test deploy <env>`",
		"#### Flags",
		"#### Positional Arguments",
		"#### Examples",
		"`--force` / `-f`",
		"`env` required target environment",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected markdown docs to contain %q, got %q", want, got)
		}
	}
}

func TestAppDocsManOutput(t *testing.T) {
	app := NewApp("test")
	var output bytes.Buffer
	app.Out = &output
	app.Err = &output
	app.Commands = []*Command{
		{
			Name:      "deploy",
			UsageLine: "test deploy <env>",
			Short:     "deploy service",
			SetFlags: func(f *FlagSet) {
				var force bool
				f.BoolVar(&force, "force", false, "force deploy", "f")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	if err := app.Run(context.Background(), []string{"docs", "man"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := output.String()
	for _, want := range []string{
		".TH TEST 1",
		".SH NAME",
		"test \\- Command\\-line tool",
		".SH COMMANDS",
		".SS test deploy",
		"\\fB--force\\fR",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected man docs to contain %q, got %q", want, got)
		}
	}
}

func TestAppDocsMarkdownBundleOutput(t *testing.T) {
	app := NewApp("test")
	var output bytes.Buffer
	app.Out = &output
	app.Err = &output
	app.Commands = []*Command{
		{
			Name:      "admin",
			UsageLine: "test admin",
			Short:     "admin tools",
			SubCommands: []*Command{
				{
					Name:      "users",
					UsageLine: "test admin users",
					Short:     "manage users",
					Run: func(ctx context.Context, cmd *Command, args []string) error {
						return nil
					},
				},
			},
		},
	}

	dir := t.TempDir()
	if err := app.Run(context.Background(), []string{"docs", "markdown", dir}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	root, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("read root markdown docs: %v", err)
	}
	commandDoc, err := os.ReadFile(filepath.Join(dir, "commands", "admin.md"))
	if err != nil {
		t.Fatalf("read command markdown docs: %v", err)
	}
	subcommandDoc, err := os.ReadFile(filepath.Join(dir, "commands", "admin", "users.md"))
	if err != nil {
		t.Fatalf("read subcommand markdown docs: %v", err)
	}

	if !strings.Contains(string(root), "commands/admin.md") || !strings.Contains(string(root), "commands/admin/users.md") {
		t.Fatalf("expected root markdown index links, got %q", string(root))
	}
	if !strings.Contains(string(commandDoc), `kind: "command"`) || !strings.Contains(string(commandDoc), `command_name: "admin"`) {
		t.Fatalf("expected command markdown frontmatter, got %q", string(commandDoc))
	}
	if !strings.Contains(string(commandDoc), "#### Subcommands") || !strings.Contains(string(commandDoc), "[`users`](admin/users.md)") {
		t.Fatalf("expected command markdown subcommand links, got %q", string(commandDoc))
	}
	if !strings.Contains(string(subcommandDoc), `command_path:`) || !strings.Contains(string(subcommandDoc), `- "test"`) || !strings.Contains(string(subcommandDoc), "# `test admin users`") {
		t.Fatalf("expected nested command markdown header, got %q", string(subcommandDoc))
	}
}

func TestAppDocsManBundleOutput(t *testing.T) {
	app := NewApp("test")
	var output bytes.Buffer
	app.Out = &output
	app.Err = &output
	app.Commands = []*Command{
		{
			Name:      "admin",
			UsageLine: "test admin",
			SubCommands: []*Command{
				{
					Name:      "users",
					UsageLine: "test admin users",
					Run: func(ctx context.Context, cmd *Command, args []string) error {
						return nil
					},
				},
			},
		},
	}

	dir := t.TempDir()
	if err := app.Run(context.Background(), []string{"docs", "man", dir}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, path := range []string{
		filepath.Join(dir, "test.1"),
		filepath.Join(dir, "test-admin.1"),
		filepath.Join(dir, "test-admin-users.1"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected man bundle file %s: %v", path, err)
		}
	}
}

func TestAppConfigBindingPrecedence(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "app.json")
	if err := os.WriteFile(configPath, []byte(`{"name":"from-config"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	app := NewApp("test")
	app.EnableConfigSupport()
	app.Err = &bytes.Buffer{}

	var name string
	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				f.StringVar(&name, "name", "", "name value", "n")
				f.BindConfig("name", "name")
				f.BindEnv("name", "APP_NAME")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	if err := app.Run(context.Background(), []string{"--config", configPath, "sayhi"}); err != nil {
		t.Fatalf("unexpected error with config: %v", err)
	}
	if name != "from-config" {
		t.Fatalf("expected config value, got %q", name)
	}

	t.Setenv("APP_NAME", "from-env")
	if err := app.Run(context.Background(), []string{"--config", configPath, "sayhi"}); err != nil {
		t.Fatalf("unexpected error with env override: %v", err)
	}
	if name != "from-env" {
		t.Fatalf("expected env override, got %q", name)
	}

	if err := app.Run(context.Background(), []string{"--config", configPath, "sayhi", "--name", "from-cli"}); err != nil {
		t.Fatalf("unexpected error with cli override: %v", err)
	}
	if name != "from-cli" {
		t.Fatalf("expected cli override, got %q", name)
	}
}

func TestAppRequiredPositionalArgumentMissing(t *testing.T) {
	app := NewApp("test")
	app.Err = &bytes.Buffer{}
	app.Commands = []*Command{
		{
			Name: "deploy",
			Positionals: []PositionalArg{
				{Name: "env", Required: true},
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"deploy"})
	if err == nil {
		t.Fatal("expected required positional validation error")
	}
	if !strings.Contains(err.Error(), "missing required argument: env") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppTooManyPositionalArguments(t *testing.T) {
	app := NewApp("test")
	app.Err = &bytes.Buffer{}
	app.Commands = []*Command{
		{
			Name: "deploy",
			Positionals: []PositionalArg{
				{Name: "env", Required: true},
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"deploy", "dev", "extra"})
	if err == nil {
		t.Fatal("expected too many positional arguments error")
	}
	if !strings.Contains(err.Error(), "too many arguments") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppRuntimeErrorNormalized(t *testing.T) {
	app := NewApp("test")
	app.Err = &bytes.Buffer{}
	app.Commands = []*Command{
		{
			Name: "boom",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return errors.New("boom failed")
			},
		},
	}

	err := app.Run(context.Background(), []string{"boom"})
	if err == nil {
		t.Fatal("expected runtime error")
	}

	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Kind != ErrorKindRuntime || cliErr.ExitCode != 1 || cliErr.Command != "boom" {
		t.Fatalf("unexpected cli error: %+v", cliErr)
	}
	if app.ExitStatus() != 1 {
		t.Fatalf("expected exit status 1, got %d", app.ExitStatus())
	}
}

func TestAppInvalidArgumentsNormalized(t *testing.T) {
	app := NewApp("test")
	app.Err = &bytes.Buffer{}
	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				var verbose bool
				f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"sayhi", "--verboes"})
	if err == nil {
		t.Fatal("expected invalid argument error")
	}

	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Kind != ErrorKindInvalidArguments || cliErr.ExitCode != 2 || cliErr.Command != "sayhi" {
		t.Fatalf("unexpected cli error: %+v", cliErr)
	}
}

func TestAppHooksOrderAndErrorHook(t *testing.T) {
	app := NewApp("test")
	app.Err = &bytes.Buffer{}
	events := make([]string, 0)
	app.BeforeRun = func(ctx HookContext) error {
		events = append(events, "app-before:"+ctx.Command.Name)
		return nil
	}
	app.AfterRun = func(ctx HookContext) {
		if ctx.Err != nil {
			events = append(events, "app-after-error:"+ctx.Command.Name)
			return
		}
		events = append(events, "app-after:"+ctx.Command.Name)
	}
	app.OnError = func(ctx HookContext) {
		var cliErr *CLIError
		if errors.As(ctx.Err, &cliErr) {
			events = append(events, "app-error:"+cliErr.Kind)
		}
	}
	app.Commands = []*Command{
		{
			Name: "deploy",
			BeforeRun: func(ctx HookContext) error {
				events = append(events, "cmd-before:"+ctx.Command.Name)
				return nil
			},
			AfterRun: func(ctx HookContext) {
				if ctx.Err != nil {
					events = append(events, "cmd-after-error:"+ctx.Command.Name)
					return
				}
				events = append(events, "cmd-after:"+ctx.Command.Name)
			},
			OnError: func(ctx HookContext) {
				var cliErr *CLIError
				if errors.As(ctx.Err, &cliErr) {
					events = append(events, "cmd-error:"+cliErr.Kind)
				}
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				events = append(events, "run:"+cmd.Name)
				return errors.New("deploy failed")
			},
		},
	}

	err := app.Run(context.Background(), []string{"deploy"})
	if err == nil {
		t.Fatal("expected error")
	}

	want := []string{
		"app-before:deploy",
		"cmd-before:deploy",
		"run:deploy",
		"cmd-error:runtime",
		"app-error:runtime",
		"cmd-after-error:deploy",
		"app-after-error:deploy",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("unexpected hook order:\nwant %v\ngot  %v", want, events)
	}
}

func TestAppUnknownFlagSuggestions(t *testing.T) {
	app := NewApp("test")
	var stderr bytes.Buffer
	app.Err = &stderr
	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				var verbose bool
				f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"sayhi", "--verboes"})
	if err == nil {
		t.Fatal("expected unknown flag error")
	}

	if !strings.Contains(err.Error(), "Did you mean --verbose?") {
		t.Fatalf("expected suggestion in error, got %v", err)
	}
}

func TestAppFlagEnvBindingAndRequiredValidation(t *testing.T) {
	t.Setenv("APP_NAME", "sam")

	app := NewApp("test")
	app.Err = &bytes.Buffer{}
	var name string
	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				f.StringVar(&name, "name", "", "name value", "n")
				f.BindEnv("name", "APP_NAME")
				f.MarkRequired("name")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"sayhi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if name != "sam" {
		t.Fatalf("expected name from env, got %q", name)
	}
}

func TestAppRequiredFlagMissing(t *testing.T) {
	app := NewApp("test")
	var stderr bytes.Buffer
	app.Err = &stderr
	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				var name string
				f.StringVar(&name, "name", "", "name value", "n")
				f.MarkRequired("name")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"sayhi"})
	if err == nil {
		t.Fatal("expected required flag validation error")
	}

	if !strings.Contains(err.Error(), "required flag(s) not set: --name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppSuggestions(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{Name: "status"},
		{Name: "commit"},
	}

	err := app.Run(context.Background(), []string{"statu"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}

	if !strings.Contains(err.Error(), "Did you mean status?") {
		t.Errorf("expected suggestion in error, got: %v", err)
	}
}

func TestAppContextCancellation(t *testing.T) {
	app := NewApp("test")
	app.Err = &bytes.Buffer{}
	app.Commands = []*Command{
		{
			Name: "long",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := app.Run(ctx, []string{"long"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Kind != ErrorKindCanceled || cliErr.ExitCode != 130 || cliErr.Command != "long" {
		t.Fatalf("unexpected cli error: %+v", cliErr)
	}
	if app.ExitStatus() != 130 {
		t.Fatalf("expected exit status 130, got %d", app.ExitStatus())
	}
}

func TestAppBeforeHookAbort(t *testing.T) {
	app := NewApp("test")
	app.Err = &bytes.Buffer{}
	events := make([]string, 0)
	app.BeforeRun = func(ctx HookContext) error {
		events = append(events, "app-before:"+ctx.Command.Name)
		return errors.New("blocked by app hook")
	}
	app.AfterRun = func(ctx HookContext) {
		if ctx.Err != nil {
			events = append(events, "app-after-error:"+ctx.Command.Name)
		}
	}
	app.OnError = func(ctx HookContext) {
		var cliErr *CLIError
		if errors.As(ctx.Err, &cliErr) {
			events = append(events, "app-error:"+cliErr.Kind)
		}
	}
	app.Commands = []*Command{
		{
			Name: "deploy",
			BeforeRun: func(ctx HookContext) error {
				events = append(events, "cmd-before:"+ctx.Command.Name)
				return nil
			},
			AfterRun: func(ctx HookContext) {
				if ctx.Err != nil {
					events = append(events, "cmd-after-error:"+ctx.Command.Name)
				}
			},
			OnError: func(ctx HookContext) {
				var cliErr *CLIError
				if errors.As(ctx.Err, &cliErr) {
					events = append(events, "cmd-error:"+cliErr.Kind)
				}
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				events = append(events, "run:"+cmd.Name)
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"deploy"})
	if err == nil {
		t.Fatal("expected error")
	}

	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Kind != ErrorKindRuntime || cliErr.ExitCode != 1 || cliErr.Command != "deploy" {
		t.Fatalf("unexpected cli error: %+v", cliErr)
	}

	want := []string{
		"app-before:deploy",
		"cmd-error:runtime",
		"app-error:runtime",
		"cmd-after-error:deploy",
		"app-after-error:deploy",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("unexpected hook order:\nwant %v\ngot  %v", want, events)
	}
}

func TestAppMiddlewareOrder(t *testing.T) {
	app := NewApp("test")
	app.Err = &bytes.Buffer{}
	events := make([]string, 0)
	app.Use(func(ctx MiddlewareContext, next NextFunc) error {
		events = append(events, "app-before:"+ctx.Command.Name)
		err := next(ctx.Context)
		events = append(events, "app-after:"+ctx.Command.Name)
		return err
	})
	app.Commands = []*Command{
		{
			Name: "deploy",
			Middlewares: []Middleware{
				func(ctx MiddlewareContext, next NextFunc) error {
					events = append(events, "cmd-before:"+ctx.Command.Name)
					err := next(ctx.Context)
					events = append(events, "cmd-after:"+ctx.Command.Name)
					return err
				},
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				events = append(events, "run:"+cmd.Name)
				return nil
			},
		},
	}

	if err := app.Run(context.Background(), []string{"deploy"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"app-before:deploy",
		"cmd-before:deploy",
		"run:deploy",
		"cmd-after:deploy",
		"app-after:deploy",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("unexpected middleware order:\nwant %v\ngot  %v", want, events)
	}
}

func TestAppObserversReceiveLifecycleEvents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		app := NewApp("test")
		app.Err = &bytes.Buffer{}
		events := make([]string, 0)
		app.AddObserver(ObserverFunc(func(event Event) {
			events = append(events, fmt.Sprintf("app:%s:%s:%d", event.Type, event.Command.Name, event.ExitCode))
		}))
		app.Commands = []*Command{
			{
				Name: "deploy",
				Observers: []Observer{
					ObserverFunc(func(event Event) {
						events = append(events, fmt.Sprintf("cmd:%s:%s:%d", event.Type, event.Command.Name, event.ExitCode))
					}),
				},
				Run: func(ctx context.Context, cmd *Command, args []string) error {
					return nil
				},
			},
		}

		if err := app.Run(context.Background(), []string{"deploy"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{
			"app:command_started:deploy:0",
			"cmd:command_started:deploy:0",
			"app:command_finished:deploy:0",
			"cmd:command_finished:deploy:0",
		}
		if !slices.Equal(events, want) {
			t.Fatalf("unexpected observer events:\nwant %v\ngot  %v", want, events)
		}
	})

	t.Run("failure", func(t *testing.T) {
		app := NewApp("test")
		app.Err = &bytes.Buffer{}
		events := make([]string, 0)
		app.AddObserver(ObserverFunc(func(event Event) {
			events = append(events, fmt.Sprintf("app:%s:%s:%d", event.Type, event.Command.Name, event.ExitCode))
		}))
		app.Commands = []*Command{
			{
				Name: "deploy",
				Observers: []Observer{
					ObserverFunc(func(event Event) {
						events = append(events, fmt.Sprintf("cmd:%s:%s:%d", event.Type, event.Command.Name, event.ExitCode))
					}),
				},
				Run: func(ctx context.Context, cmd *Command, args []string) error {
					return errors.New("deploy failed")
				},
			},
		}

		err := app.Run(context.Background(), []string{"deploy"})
		if err == nil {
			t.Fatal("expected error")
		}

		want := []string{
			"app:command_started:deploy:0",
			"cmd:command_started:deploy:0",
			"app:command_failed:deploy:1",
			"cmd:command_failed:deploy:1",
		}
		if !slices.Equal(events, want) {
			t.Fatalf("unexpected observer events:\nwant %v\ngot  %v", want, events)
		}
	})
}

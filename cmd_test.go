package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	want := []string{"--verbose", "-v", "sayhi", "status"}
	for _, expected := range want {
		if !slices.Contains(got, expected) {
			t.Fatalf("expected %q in suggestions, got %v", expected, got)
		}
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
				{Name: "target", Usage: "target subject", Required: true, Enum: []string{"team", "user"}},
			},
			SetFlags: func(f *FlagSet) {
				var name string
				f.StringVar(&name, "name", "", "name value", "n")
				f.BindEnv("name", "APP_NAME")
				f.BindConfig("name", "name")
				f.SetEnum("name", "sam", "sara")
				f.MarkRequired("name")
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

	if !slices.ContainsFunc(spec.GlobalFlags, func(flag FlagSpec) bool { return flag.Name == "verbose" }) {
		t.Fatalf("expected global flags in spec, got %+v", spec.GlobalFlags)
	}

	if !slices.ContainsFunc(spec.GlobalFlags, func(flag FlagSpec) bool { return flag.Name == "config" }) {
		t.Fatalf("expected builtin config flag in spec, got %+v", spec.GlobalFlags)
	}

	if len(spec.Commands) != 1 || spec.Commands[0].Name != "sayhi" {
		t.Fatalf("expected sayhi command in spec, got %+v", spec.Commands)
	}

	if !slices.ContainsFunc(spec.Commands[0].Flags, func(flag FlagSpec) bool {
		return flag.Name == "name" && flag.Required && len(flag.ConfigKeys) == 1 && flag.ConfigKeys[0] == "name" && len(flag.Enum) == 2
	}) {
		t.Fatalf("expected required name flag in spec, got %+v", spec.Commands[0].Flags)
	}

	if len(spec.Commands[0].Positionals) != 1 || spec.Commands[0].Positionals[0].Name != "target" || len(spec.Commands[0].Positionals[0].Enum) != 2 {
		t.Fatalf("expected positional spec, got %+v", spec.Commands[0].Positionals)
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
}

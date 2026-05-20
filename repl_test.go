package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestSplitLine(t *testing.T) {
	got, err := SplitLine("foo bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"foo", "bar"}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSplitLineQuotes(t *testing.T) {
	got, err := SplitLine(`foo "hello world" 'good bye'`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"foo", "hello world", "good bye"}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSplitLineEscapes(t *testing.T) {
	got, err := SplitLine(`foo a\ b \"bar\"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"foo", "a b", `"bar"`}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSplitLineUnclosedQuote(t *testing.T) {
	if _, err := SplitLine(`foo "bar`); err == nil {
		t.Fatal("expected unclosed quote error")
	}
}

func TestSplitLineForCompletion(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantArgs    []string
		wantCurrent string
	}{
		{
			name:        "partial command",
			line:        "user cr",
			wantArgs:    []string{"user"},
			wantCurrent: "cr",
		},
		{
			name:        "trailing space",
			line:        "user create ",
			wantArgs:    []string{"user", "create"},
			wantCurrent: "",
		},
		{
			name:        "root flag",
			line:        "--ver",
			wantArgs:    nil,
			wantCurrent: "--ver",
		},
		{
			name:        "flag value",
			line:        "deploy --env p",
			wantArgs:    []string{"deploy", "--env"},
			wantCurrent: "p",
		},
		{
			name:        "partial quoted token",
			line:        `deploy "pro`,
			wantArgs:    []string{"deploy"},
			wantCurrent: "pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgs, gotCurrent, err := SplitLineForCompletion(tt.line)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(gotArgs, tt.wantArgs) {
				t.Fatalf("expected args %v, got %v", tt.wantArgs, gotArgs)
			}
			if gotCurrent != tt.wantCurrent {
				t.Fatalf("expected current %q, got %q", tt.wantCurrent, gotCurrent)
			}
		})
	}
}

func TestSplitLineForCompletionUnclosedDoubleQuote(t *testing.T) {
	gotArgs, gotCurrent, err := SplitLineForCompletion(`foo "bar baz`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(gotArgs, []string{"foo"}) {
		t.Fatalf("expected args [foo], got %v", gotArgs)
	}
	if gotCurrent != "bar baz" {
		t.Fatalf("expected current %q, got %q", "bar baz", gotCurrent)
	}
}

func TestSplitLineForCompletionUnclosedSingleQuote(t *testing.T) {
	gotArgs, gotCurrent, err := SplitLineForCompletion(`foo 'bar baz`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(gotArgs, []string{"foo"}) {
		t.Fatalf("expected args [foo], got %v", gotArgs)
	}
	if gotCurrent != "bar baz" {
		t.Fatalf("expected current %q, got %q", "bar baz", gotCurrent)
	}
}

func TestSplitLineForCompletionFlagEquals(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantArgs    []string
		wantCurrent string
	}{
		{
			name:        "attached equals",
			line:        "deploy --env=pr",
			wantArgs:    []string{"deploy"},
			wantCurrent: "--env=pr",
		},
		{
			name:        "attached equals with quote",
			line:        `foo --name="sa`,
			wantArgs:    []string{"foo"},
			wantCurrent: "--name=sa",
		},
		{
			name:        "attached equals with single quote",
			line:        `foo --name='sa`,
			wantArgs:    []string{"foo"},
			wantCurrent: "--name=sa",
		},
		{
			name:        "separate value",
			line:        "deploy --env pr",
			wantArgs:    []string{"deploy", "--env"},
			wantCurrent: "pr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgs, gotCurrent, err := SplitLineForCompletion(tt.line)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(gotArgs, tt.wantArgs) {
				t.Fatalf("expected args %v, got %v", tt.wantArgs, gotArgs)
			}
			if gotCurrent != tt.wantCurrent {
				t.Fatalf("expected current %q, got %q", tt.wantCurrent, gotCurrent)
			}
		})
	}
}

func TestSplitLineForCompletionAfterDoubleDash(t *testing.T) {
	gotArgs, gotCurrent, err := SplitLineForCompletion(`foo -- --not-a`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(gotArgs, []string{"foo", "--"}) {
		t.Fatalf("expected args %v, got %v", []string{"foo", "--"}, gotArgs)
	}
	if gotCurrent != "--not-a" {
		t.Fatalf("expected current %q, got %q", "--not-a", gotCurrent)
	}
}

func TestSplitLineForCompletionTrailingSpace(t *testing.T) {
	gotArgs, gotCurrent, err := SplitLineForCompletion(`foo --name sa `)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(gotArgs, []string{"foo", "--name", "sa"}) {
		t.Fatalf("expected args %v, got %v", []string{"foo", "--name", "sa"}, gotArgs)
	}
	if gotCurrent != "" {
		t.Fatalf("expected empty current, got %q", gotCurrent)
	}
}

func TestAppRunLineReusesCommands(t *testing.T) {
	app := NewApp("test")
	var gotArgs []string

	app.Commands = []*Command{
		{
			Name: "admin",
			SubCommands: []*Command{
				{
					Name: "users",
					Run: func(ctx context.Context, cmd *Command, args []string) error {
						gotArgs = append([]string(nil), args...)
						return nil
					},
				},
			},
		},
	}

	if err := app.RunLine(context.Background(), `admin users "sam lee"`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"sam lee"}
	if !slices.Equal(gotArgs, want) {
		t.Fatalf("expected %v, got %v", want, gotArgs)
	}
}

func TestRunLineDoesNotLeakState(t *testing.T) {
	app := NewApp("test")
	app.EnableConfigSupport()

	var verbose bool
	var observed []bool
	var name string

	app.SetFlags = func(f *FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
	}
	app.Commands = []*Command{
		{
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				f.StringVar(&name, "name", "", "name value", "n")
				f.MarkRequired("name")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				observed = append(observed, verbose)
				return nil
			},
		},
	}

	if err := app.RunLine(context.Background(), `--verbose sayhi --name sam`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := app.RunLine(context.Background(), `sayhi --name tom`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !slices.Equal(observed, []bool{true, false}) {
		t.Fatalf("expected isolated verbose state, got %v", observed)
	}
	if app.ExitStatus() != 0 {
		t.Fatalf("expected exit status 0 after successful run, got %d", app.ExitStatus())
	}
	if app.flag == nil {
		t.Fatal("expected current run to initialize app flag set")
	}
}

func TestRunLineAfterErrorStillWorks(t *testing.T) {
	app := NewApp("test")
	var ran bool

	app.Commands = []*Command{
		{
			Name: "boom",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return errors.New("boom")
			},
		},
		{
			Name: "ok",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				ran = true
				return nil
			},
		},
	}

	err := app.RunLine(context.Background(), "boom")
	if err == nil {
		t.Fatal("expected error from boom command")
	}
	if app.ExitStatus() != 1 {
		t.Fatalf("expected exit status 1 after failing run, got %d", app.ExitStatus())
	}

	if err := app.RunLine(context.Background(), "ok"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Fatal("expected ok command to run after previous error")
	}
	if app.ExitStatus() != 0 {
		t.Fatalf("expected exit status reset after successful run, got %d", app.ExitStatus())
	}
}

func TestAppCompleteLineReusesExistingCompletion(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "deploy",
			Positionals: []PositionalArg{
				{Name: "env", Enum: []string{"dev", "prod"}},
			},
			SetFlags: func(f *FlagSet) {
				var env string
				f.StringVar(&env, "env", "", "target environment", "")
				f.SetEnum("env", "dev", "prod")
			},
		},
	}

	commandSuggestions := app.CompleteLine("dep", len("dep"))
	if !slices.Contains(commandSuggestions, "deploy") {
		t.Fatalf("expected deploy command suggestion, got %v", commandSuggestions)
	}

	flagValueSuggestions := app.CompleteLine("deploy --env p", len("deploy --env p"))
	if !slices.Equal(flagValueSuggestions, []string{"prod"}) {
		t.Fatalf("expected prod flag suggestion, got %v", flagValueSuggestions)
	}

	positionalSuggestions := app.CompleteLine("deploy d", len("deploy d"))
	if !slices.Equal(positionalSuggestions, []string{"dev"}) {
		t.Fatalf("expected dev positional suggestion, got %v", positionalSuggestions)
	}
}

func TestCompleteLineDetailedCommands(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{Name: "deploy", Short: "deploy services"},
	}

	got := app.CompleteLineDetailed("dep", len("dep"))
	want := []CompletionResult{
		{Value: "deploy", Description: "deploy services", Kind: completionKindCommand},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompleteLineDetailedFlags(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "deploy",
			SetFlags: func(f *FlagSet) {
				var env string
				f.StringVar(&env, "env", "", "target environment", "e")
			},
		},
	}

	got := app.CompleteLineDetailed("deploy --e", len("deploy --e"))
	want := []CompletionResult{
		{Value: "--env", Description: "target environment", Kind: completionKindFlag},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompleteLineDetailedBuiltins(t *testing.T) {
	app := NewApp("test")

	got := app.CompleteLineDetailed("sp", len("sp"))
	want := []CompletionResult{
		{Value: "spec", Kind: completionKindBuiltin},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompleteLineDetailedPositional(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "deploy",
			Positionals: []PositionalArg{
				{Name: "env", Enum: []string{"dev", "prod"}},
			},
		},
	}

	got := app.CompleteLineDetailed("deploy d", len("deploy d"))
	want := []CompletionResult{
		{Value: "dev", Kind: completionKindPositional},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestCompleteLineCompatibility(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name:  "deploy",
			Short: "deploy services",
			SetFlags: func(f *FlagSet) {
				var env string
				f.StringVar(&env, "env", "", "target environment", "")
			},
		},
	}

	gotStrings := app.CompleteLine("dep", len("dep"))
	gotDetailed := app.CompleteLineDetailed("dep", len("dep"))
	if !slices.Equal(gotStrings, completionValues(gotDetailed)) {
		t.Fatalf("expected CompleteLine compatibility, got strings=%v detailed=%v", gotStrings, gotDetailed)
	}
}

func TestShellCompletionStillPlainText(t *testing.T) {
	app := NewApp("test")
	var output bytes.Buffer
	app.Out = &output
	app.Err = &output
	app.Commands = []*Command{
		{Name: "deploy", Short: "deploy services"},
	}

	if err := app.Run(context.Background(), []string{"__complete", "dep"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := strings.TrimSpace(output.String()); got != "deploy" {
		t.Fatalf("expected plain text shell completion, got %q", output.String())
	}
}

func TestCompleteLineDetailedDescriptions(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name:  "deploy",
			Short: "deploy services",
			SetFlags: func(f *FlagSet) {
				var env string
				f.StringVar(&env, "env", "", "target environment", "")
			},
		},
	}

	commandResults := app.CompleteLineDetailed("dep", len("dep"))
	if len(commandResults) != 1 || commandResults[0].Description != "deploy services" {
		t.Fatalf("expected command description, got %v", commandResults)
	}

	flagResults := app.CompleteLineDetailed("deploy --e", len("deploy --e"))
	if len(flagResults) != 1 || flagResults[0].Description != "target environment" {
		t.Fatalf("expected flag description, got %v", flagResults)
	}
}

func TestCompleteLineCursorMiddleOfLine(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "user",
			SubCommands: []*Command{
				{
					Name: "create",
					SetFlags: func(f *FlagSet) {
						var name string
						f.StringVar(&name, "name", "", "name value", "n")
					},
				},
			},
		},
	}

	line := "user create sam --na foo"
	cursor := len("user create sam --na")

	got := app.CompleteLine(line, cursor)
	if !slices.Equal(got, []string{"--name"}) {
		t.Fatalf("expected --name completion from cursor slice, got %v", got)
	}
}

func TestREPLExit(t *testing.T) {
	app := NewApp("test")
	var out bytes.Buffer

	repl := &REPL{
		App: app,
		In:  strings.NewReader("exit\n"),
		Out: &out,
		Err: &out,
	}

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := out.String(); got != "> " {
		t.Fatalf("expected single prompt, got %q", got)
	}
}

func TestREPLContinuesAfterCommandError(t *testing.T) {
	app := NewApp("test")
	var ranOK bool

	app.Commands = []*Command{
		{
			Name: "fail",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return errors.New("boom")
			},
		},
		{
			Name: "ok",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				ranOK = true
				fmt.Fprintln(app.Out, "ok")
				return nil
			},
		},
	}

	var out bytes.Buffer
	repl := &REPL{
		App: app,
		In:  strings.NewReader("fail\nok\nexit\n"),
		Out: &out,
		Err: &out,
	}

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ranOK {
		t.Fatal("expected ok command to run after failure")
	}

	output := out.String()
	if !strings.Contains(output, "Error: boom") {
		t.Fatalf("expected error output, got %q", output)
	}
	if !strings.Contains(output, "ok\n") {
		t.Fatalf("expected successful command output, got %q", output)
	}
}

func TestREPLMultipleCommandsIsolation(t *testing.T) {
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
				f.MarkRequired("name")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				fmt.Fprintf(app.Out, "verbose=%t name=%s\n", verbose, name)
				return nil
			},
		},
	}

	var out bytes.Buffer
	repl := &REPL{
		App: app,
		In:  strings.NewReader("--verbose sayhi --name sam\nsayhi --name tom\nexit\n"),
		Out: &out,
		Err: &out,
	}

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "verbose=true name=sam") {
		t.Fatalf("expected first command output, got %q", output)
	}
	if !strings.Contains(output, "verbose=false name=tom") {
		t.Fatalf("expected second command isolation output, got %q", output)
	}
}

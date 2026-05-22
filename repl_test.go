package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"pkg.gostartkit.com/cmd/internal/terminal"
	"slices"
	"strings"
	"testing"
)

type replDriverFunc func(ctx context.Context, repl *REPL) error

func (fn replDriverFunc) Run(ctx context.Context, repl *REPL) error {
	return fn(ctx, repl)
}

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

func TestREPLBuiltinUsesConfiguredDriver(t *testing.T) {
	app := NewApp("test")
	app.EnableREPL()

	ran := false
	app.ConfigureREPL(func(cfg *REPLConfig) {
		cfg.Prompt = "test> "
		cfg.Driver = replDriverFunc(func(ctx context.Context, repl *REPL) error {
			ran = true
			if repl.App != app {
				t.Fatalf("expected repl app to match")
			}
			if repl.Prompt != "test> " {
				t.Fatalf("expected configured prompt, got %q", repl.Prompt)
			}
			return nil
		})
	})

	if err := app.Run(context.Background(), []string{"repl"}); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Fatal("expected configured repl driver to run")
	}
}

func TestREPLPromptFallbacks(t *testing.T) {
	repl := &REPL{}
	if got := repl.prompt(context.Background()); got != "> " {
		t.Fatalf("expected default prompt, got %q", got)
	}

	repl.Prompt = "static> "
	if got := repl.prompt(context.Background()); got != "static> " {
		t.Fatalf("expected static prompt, got %q", got)
	}

	repl.PromptFunc = func(ctx context.Context, repl *REPL) string {
		return "dynamic> "
	}
	if got := repl.prompt(context.Background()); got != "dynamic> " {
		t.Fatalf("expected dynamic prompt, got %q", got)
	}

	repl.PromptFunc = func(ctx context.Context, repl *REPL) string {
		return ""
	}
	if got := repl.prompt(context.Background()); got != "static> " {
		t.Fatalf("expected prompt fallback to static prompt, got %q", got)
	}

	repl.Prompt = ""
	if got := repl.prompt(context.Background()); got != "> " {
		t.Fatalf("expected prompt fallback to default prompt, got %q", got)
	}
}

func TestBasicREPLPromptFuncRunsEachRender(t *testing.T) {
	app := NewApp("test")
	var count int

	repl := &REPL{
		App: app,
		In:  strings.NewReader("\nexit\n"),
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
		PromptFunc: func(ctx context.Context, repl *REPL) string {
			count++
			return fmt.Sprintf("%d> ", count)
		},
	}
	out := repl.Out.(*bytes.Buffer)

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected prompt func to run twice, got %d", count)
	}
	if got := out.String(); got != "1> 2> " {
		t.Fatalf("unexpected prompt output: %q", got)
	}
}

func TestREPLHistoryLoadHookRunsBeforeDriver(t *testing.T) {
	app := NewApp("test")
	var loaded bool
	var ran bool

	repl := &REPL{
		App: app,
		History: &REPLHistoryHooks{
			Load: func(ctx context.Context) ([]string, error) {
				loaded = true
				return []string{"deploy prod", "deploy staging"}, nil
			},
		},
		Driver: replDriverFunc(func(ctx context.Context, repl *REPL) error {
			ran = true
			if !slices.Equal(repl.loadedHistory, []string{"deploy prod", "deploy staging"}) {
				t.Fatalf("unexpected loaded history: %v", repl.loadedHistory)
			}
			return nil
		}),
	}

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !loaded || !ran {
		t.Fatalf("expected load and driver to run, loaded=%v ran=%v", loaded, ran)
	}
}

func TestREPLHistoryLoadHookErrorStopsStartup(t *testing.T) {
	app := NewApp("test")
	ran := false
	repl := &REPL{
		App: app,
		History: &REPLHistoryHooks{
			Load: func(ctx context.Context) ([]string, error) {
				return nil, errors.New("load failed")
			},
		},
		Driver: replDriverFunc(func(ctx context.Context, repl *REPL) error {
			ran = true
			return nil
		}),
	}

	err := repl.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "repl history load: load failed") {
		t.Fatalf("expected wrapped load error, got %v", err)
	}
	if ran {
		t.Fatal("expected driver not to run when history load fails")
	}
}

func TestBasicREPLHistoryAppendHookRunsOnAcceptedLines(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "hello",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	var got []string
	var out bytes.Buffer
	repl := &REPL{
		App: app,
		In:  strings.NewReader("hello\nexit\n"),
		Out: &out,
		Err: &out,
		History: &REPLHistoryHooks{
			Append: func(ctx context.Context, line string) error {
				got = append(got, line)
				return nil
			},
		},
	}

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(got, []string{"hello", "exit"}) {
		t.Fatalf("unexpected appended history lines: %v", got)
	}
}

func TestBasicREPLHistoryAppendHookWarningDoesNotBlockCommand(t *testing.T) {
	app := NewApp("test")
	ran := false
	app.Commands = []*Command{
		{
			Name: "hello",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				ran = true
				fmt.Fprintln(app.Out, "ok")
				return nil
			},
		},
	}

	var out bytes.Buffer
	repl := &REPL{
		App: app,
		In:  strings.NewReader("hello\nexit\n"),
		Out: &out,
		Err: &out,
		History: &REPLHistoryHooks{
			Append: func(ctx context.Context, line string) error {
				if line == "hello" {
					return errors.New("append failed")
				}
				return nil
			},
		},
	}

	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Fatal("expected command to run even if append hook fails")
	}
	output := out.String()
	if !strings.Contains(output, "Warning: repl history append: append failed") {
		t.Fatalf("expected append warning, got %q", output)
	}
	if !strings.Contains(output, "ok\n") {
		t.Fatalf("expected command output, got %q", output)
	}
}

func TestCompletionReplaceStart(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		cursor      int
		wantStart   int
		wantCurrent string
	}{
		{
			name:        "plain token",
			line:        "deploy pr",
			cursor:      len([]rune("deploy pr")),
			wantStart:   len([]rune("deploy ")),
			wantCurrent: "pr",
		},
		{
			name:        "quoted token",
			line:        `deploy "pr`,
			cursor:      len([]rune(`deploy "pr`)),
			wantStart:   len([]rune(`deploy "`)),
			wantCurrent: "pr",
		},
		{
			name:        "trailing space",
			line:        "deploy ",
			cursor:      len([]rune("deploy ")),
			wantStart:   len([]rune("deploy ")),
			wantCurrent: "",
		},
		{
			name:        "flag equals",
			line:        "deploy --env=pr",
			cursor:      len([]rune("deploy --env=pr")),
			wantStart:   len([]rune("deploy ")),
			wantCurrent: "--env=pr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotCurrent := completionReplaceStart([]rune(tt.line), tt.cursor)
			if gotStart != tt.wantStart || gotCurrent != tt.wantCurrent {
				t.Fatalf("expected start=%d current=%q, got start=%d current=%q", tt.wantStart, tt.wantCurrent, gotStart, gotCurrent)
			}
		})
	}
}

func TestFormatCompletionHint(t *testing.T) {
	tests := []struct {
		name    string
		results []CompletionResult
		want    string
	}{
		{
			name: "empty",
			want: "",
		},
		{
			name: "single with description",
			results: []CompletionResult{
				{Value: "--env", Description: "target environment", Kind: completionKindFlag},
			},
			want: "hint: --env - target environment",
		},
		{
			name: "single without description",
			results: []CompletionResult{
				{Value: "deploy", Kind: completionKindCommand},
			},
			want: "hint: deploy",
		},
		{
			name: "multiple",
			results: []CompletionResult{
				{Value: "deploy"},
				{Value: "describe"},
				{Value: "destroy"},
				{Value: "docs"},
			},
			want: "hint: deploy, describe, destroy (+1 more)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatCompletionHint(tt.results); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestTerminalSessionCurrentHintUsesCompletionDetails(t *testing.T) {
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

	session := &replTerminalSession{
		repl:   &REPL{App: app},
		line:   []rune("dep"),
		cursor: len([]rune("dep")),
	}
	if got := session.currentHint(); got != "hint: deploy - deploy services" {
		t.Fatalf("unexpected command hint: %q", got)
	}

	session.line = []rune("deploy --e")
	session.cursor = len([]rune("deploy --e"))
	if got := session.currentHint(); got != "hint: --env - target environment" {
		t.Fatalf("unexpected flag hint: %q", got)
	}
}

func TestFormatCompletionGhostText(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		cursor  int
		results []CompletionResult
		want    string
	}{
		{
			name:   "empty",
			line:   "dep",
			cursor: len([]rune("dep")),
			want:   "",
		},
		{
			name:   "single suggestion",
			line:   "dep",
			cursor: len([]rune("dep")),
			results: []CompletionResult{
				{Value: "deploy", Description: "deploy services"},
			},
			want: ansiDim + "loy" + ansiReset,
		},
		{
			name:   "common prefix from multiple suggestions",
			line:   "de",
			cursor: len([]rune("de")),
			results: []CompletionResult{
				{Value: "deploy"},
				{Value: "describe"},
				{Value: "destroy"},
			},
			want: "",
		},
		{
			name:   "cursor not at end",
			line:   "deploy",
			cursor: len([]rune("dep")),
			results: []CompletionResult{
				{Value: "deploy"},
			},
			want: "",
		},
		{
			name:   "trailing space",
			line:   "deploy ",
			cursor: len([]rune("deploy ")),
			results: []CompletionResult{
				{Value: "--env"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatCompletionGhostText([]rune(tt.line), tt.cursor, tt.results); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestTerminalSessionCurrentGhostTextUsesCompletionDetails(t *testing.T) {
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

	session := &replTerminalSession{
		repl:   &REPL{App: app},
		line:   []rune("dep"),
		cursor: len([]rune("dep")),
	}
	if got := session.currentGhostText(); got != ansiDim+"loy"+ansiReset {
		t.Fatalf("unexpected command ghost text: %q", got)
	}

	session.line = []rune("deploy --e")
	session.cursor = len([]rune("deploy --e"))
	if got := session.currentGhostText(); got != ansiDim+"nv"+ansiReset {
		t.Fatalf("unexpected flag ghost text: %q", got)
	}
}

func TestTerminalSessionRenderStartsHintAtLineStart(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name:  "deploy",
			Short: "deploy services",
		},
	}

	tmp, err := os.CreateTemp(t.TempDir(), "render-out")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer tmp.Close()

	session := &replTerminalSession{
		repl: &REPL{
			App: app,
			Prompt: "cmd> ",
		},
		ctx:    context.Background(),
		out:    tmp,
		line:   []rune("dep"),
		cursor: len([]rune("dep")),
	}

	if err := session.render(); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		t.Fatalf("seek temp file: %v", err)
	}

	gotBytes, err := io.ReadAll(tmp)
	if err != nil {
		t.Fatalf("read temp file: %v", err)
	}

	got := string(gotBytes)
	want := "\r\033[2Kcmd> dep" + ansiDim + "loy" + ansiReset + "\n\r\033[2K" + ansiDim + "hint: deploy - deploy services" + ansiReset + "\033[1A\r\033[8C"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFormatCompletionDisplayLine(t *testing.T) {
	tests := []struct {
		name   string
		result CompletionResult
		want   string
	}{
		{
			name:   "command with description",
			result: CompletionResult{Value: "deploy", Description: "deploy services", Kind: completionKindCommand},
			want:   ansiBlue + "[cmd]" + ansiReset + " " + fmt.Sprintf("%-20s %s", "deploy", "deploy services"),
		},
		{
			name:   "flag with description",
			result: CompletionResult{Value: "--env", Description: "target environment", Kind: completionKindFlag},
			want:   ansiGreen + "[flag]" + ansiReset + " " + fmt.Sprintf("%-20s %s", "--env", "target environment"),
		},
		{
			name:   "value without description",
			result: CompletionResult{Value: "prod", Kind: completionKindValue},
			want:   ansiYellow + "[value]" + ansiReset + " prod",
		},
		{
			name:   "positional without description",
			result: CompletionResult{Value: "dev", Kind: completionKindPositional},
			want:   ansiCyan + "[arg]" + ansiReset + " dev",
		},
		{
			name:   "builtin without description",
			result: CompletionResult{Value: "spec", Kind: completionKindBuiltin},
			want:   ansiDim + "[builtin]" + ansiReset + " spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatCompletionDisplayLine(tt.result); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestCompletionPageHelpers(t *testing.T) {
	results := []CompletionResult{
		{Value: "one"},
		{Value: "two"},
		{Value: "three"},
		{Value: "four"},
		{Value: "five"},
	}

	page0 := completionPage(results, 0, 2)
	if got := []string{page0[0].Value, page0[1].Value}; !slices.Equal(got, []string{"one", "two"}) {
		t.Fatalf("unexpected first page: %v", got)
	}

	page1 := completionPage(results, 1, 2)
	if got := []string{page1[0].Value, page1[1].Value}; !slices.Equal(got, []string{"three", "four"}) {
		t.Fatalf("unexpected second page: %v", got)
	}

	page2 := completionPage(results, 2, 2)
	if got := []string{page2[0].Value}; !slices.Equal(got, []string{"five"}) {
		t.Fatalf("unexpected third page: %v", got)
	}

	if got := completionPageCount(len(results), 2); got != 3 {
		t.Fatalf("expected 3 pages, got %d", got)
	}

	wantFooter := ansiDim + "hint: showing 3-4 of 5 (page 2/3, Tab for more)" + ansiReset
	if got := formatCompletionPageFooter(results, 1, 2); got != wantFooter {
		t.Fatalf("expected %q, got %q", wantFooter, got)
	}
}

func TestCompletionCycleAdvancesWithinSameContext(t *testing.T) {
	session := &replTerminalSession{}
	line := "dep"
	cursor := len([]rune(line))

	if got := session.nextCompletionPage(line, cursor, 20); got != 0 {
		t.Fatalf("expected first page 0, got %d", got)
	}
	if got := session.nextCompletionPage(line, cursor, 20); got != 1 {
		t.Fatalf("expected second page 1, got %d", got)
	}
	if got := session.nextCompletionPage(line, cursor, 20); got != 2 {
		t.Fatalf("expected third page 2, got %d", got)
	}

	session.resetCompletionCycle()
	if got := session.nextCompletionPage("deploy", len([]rune("deploy")), 20); got != 0 {
		t.Fatalf("expected reset page 0, got %d", got)
	}
}

func TestTerminalSessionInitHistoryCopiesLoadedHistory(t *testing.T) {
	session := &replTerminalSession{
		repl: &REPL{
			loadedHistory: []string{"deploy prod", "deploy staging"},
		},
	}

	session.initHistory()
	if !slices.Equal(session.history, []string{"deploy prod", "deploy staging"}) {
		t.Fatalf("unexpected session history: %v", session.history)
	}

	session.history[0] = "changed"
	if session.repl.loadedHistory[0] != "deploy prod" {
		t.Fatalf("expected loaded history to stay isolated, got %v", session.repl.loadedHistory)
	}
}

func TestTerminalSessionSubmitSuspendsAndResumesRawMode(t *testing.T) {
	withTerminalHooks(t, func(state *terminalHookState) {
		in := tempFile(t)
		out := tempFile(t)
		var session *replTerminalSession

		app := NewApp("test")
		app.Commands = []*Command{
			{
				Name: "run",
				Run: func(ctx context.Context, cmd *Command, args []string) error {
					if session.rawState != nil {
						t.Fatal("expected raw mode to be suspended during command execution")
					}
					return nil
				},
			},
		}

		session = &replTerminalSession{
			repl:   &REPL{App: app, Err: io.Discard},
			ctx:    context.Background(),
			in:     in,
			out:    out,
			line:   []rune("run"),
			cursor: len([]rune("run")),
		}

		if err := session.enterRawMode(); err != nil {
			t.Fatalf("enter raw mode: %v", err)
		}
		if err := session.submit(context.Background()); err != nil {
			t.Fatalf("submit: %v", err)
		}
		if state.enterCount != 2 {
			t.Fatalf("expected raw mode to enter twice, got %d", state.enterCount)
		}
		if state.leaveCount != 1 {
			t.Fatalf("expected raw mode to leave once, got %d", state.leaveCount)
		}
		if session.rawState == nil {
			t.Fatal("expected raw mode to be restored after command execution")
		}
	})
}

func TestTerminalSessionSubmitResumesRawModeAfterCommandError(t *testing.T) {
	withTerminalHooks(t, func(state *terminalHookState) {
		in := tempFile(t)
		out := tempFile(t)
		app := NewApp("test")
		app.Commands = []*Command{
			{
				Name: "fail",
				Run: func(ctx context.Context, cmd *Command, args []string) error {
					return errors.New("boom")
				},
			},
		}

		var errOut bytes.Buffer
		session := &replTerminalSession{
			repl:   &REPL{App: app, Err: &errOut},
			ctx:    context.Background(),
			in:     in,
			out:    out,
			line:   []rune("fail"),
			cursor: len([]rune("fail")),
		}

		if err := session.enterRawMode(); err != nil {
			t.Fatalf("enter raw mode: %v", err)
		}
		if err := session.submit(context.Background()); err != nil {
			t.Fatalf("submit: %v", err)
		}
		if state.enterCount != 2 || state.leaveCount != 1 {
			t.Fatalf("unexpected raw mode transitions: enters=%d leaves=%d", state.enterCount, state.leaveCount)
		}
		if session.rawState == nil {
			t.Fatal("expected raw mode to be restored after command error")
		}
		if !strings.Contains(errOut.String(), "Error: boom") {
			t.Fatalf("expected command error output, got %q", errOut.String())
		}
	})
}

func TestTerminalSessionSubmitExitLeavesRawModeWithoutLeak(t *testing.T) {
	withTerminalHooks(t, func(state *terminalHookState) {
		in := tempFile(t)
		out := tempFile(t)
		session := &replTerminalSession{
			repl:   &REPL{App: NewApp("test"), Err: io.Discard},
			ctx:    context.Background(),
			in:     in,
			out:    out,
			line:   []rune("exit"),
			cursor: len([]rune("exit")),
		}

		if err := session.enterRawMode(); err != nil {
			t.Fatalf("enter raw mode: %v", err)
		}
		err := session.submit(context.Background())
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected EOF on exit, got %v", err)
		}
		if state.enterCount != 1 {
			t.Fatalf("expected raw mode to enter once, got %d", state.enterCount)
		}
		if state.leaveCount != 1 {
			t.Fatalf("expected raw mode to leave once, got %d", state.leaveCount)
		}
		if session.rawState != nil {
			t.Fatal("expected raw mode state to be cleared on exit")
		}
	})
}

type terminalHookState struct {
	enterCount int
	leaveCount int
}

func withTerminalHooks(t *testing.T, fn func(state *terminalHookState)) {
	t.Helper()

	state := &terminalHookState{}
	oldIsTerminal := terminalIsTerminalFD
	oldMakeRaw := terminalMakeRawFD
	oldRestore := terminalRestoreFD
	defer func() {
		terminalIsTerminalFD = oldIsTerminal
		terminalMakeRawFD = oldMakeRaw
		terminalRestoreFD = oldRestore
	}()

	terminalIsTerminalFD = func(fd int) bool { return true }
	terminalMakeRawFD = func(fd int) (*terminal.State, error) {
		state.enterCount++
		return &terminal.State{}, nil
	}
	terminalRestoreFD = func(fd int, st *terminal.State) error {
		state.leaveCount++
		return nil
	}

	fn(state)
}

func tempFile(t *testing.T) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "repl-terminal-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	t.Cleanup(func() {
		file.Close()
	})
	return file
}

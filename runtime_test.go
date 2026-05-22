package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestAppRunWithNilRuntime(t *testing.T) {
	app := NewApp("test")
	err := app.RunWith(context.Background(), nil)
	if !errors.Is(err, errNilRuntime) {
		t.Fatalf("expected errNilRuntime, got %v", err)
	}
}

func TestCLIRuntimeRunsCommand(t *testing.T) {
	app := NewApp("test")

	var got []string
	app.Commands = []*Command{
		{
			Name: "hello",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				got = append([]string(nil), args...)
				return nil
			},
		},
	}

	if err := app.RunWith(context.Background(), CLIRuntime{Args: []string{"hello", "team"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"team"}; !strings.EqualFold(strings.Join(got, ","), strings.Join(want, ",")) {
		t.Fatalf("expected args %v, got %v", want, got)
	}
}

func TestREPLRuntimeUsesOverrides(t *testing.T) {
	app := NewApp("test")

	var ran bool
	runtime := REPLRuntime{
		Prompt:  "rt> ",
		Welcome: "welcome",
		Driver: replDriverFunc(func(ctx context.Context, repl *REPL) error {
			ran = true
			if repl.Prompt != "rt> " {
				t.Fatalf("expected runtime prompt override, got %q", repl.Prompt)
			}
			if repl.Welcome != "welcome" {
				t.Fatalf("expected runtime welcome override, got %q", repl.Welcome)
			}
			return nil
		}),
	}

	if err := app.RunWith(context.Background(), runtime); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Fatal("expected repl runtime driver to run")
	}
}

func TestAutoRuntimeUsesCLIWhenArgsPresent(t *testing.T) {
	app := NewApp("test")
	app.EnableREPL()

	var ran bool
	app.Commands = []*Command{
		{
			Name: "hello",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				ran = true
				return nil
			},
		},
	}

	if err := app.RunWith(context.Background(), AutoRuntime{
		Args: []string{"hello"},
		In:   strings.NewReader(""),
		Out:  &bytes.Buffer{},
		Err:  &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Fatal("expected auto runtime to prefer CLI when args are present")
	}
}

func TestAutoRuntimeFallsBackToCLIWithoutTTY(t *testing.T) {
	app := NewApp("test")
	app.EnableREPL()

	var ran bool
	app.SetRootCommand(&Command{
		Name: "test",
		Run: func(ctx context.Context, cmd *Command, args []string) error {
			ran = true
			return nil
		},
	})

	if err := app.RunWith(context.Background(), AutoRuntime{
		In:  strings.NewReader(""),
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Fatal("expected auto runtime to fall back to CLI when IO is not a TTY")
	}
}

func TestAppDefaultRuntimeUsesAutoRuntimeWithStandardStreams(t *testing.T) {
	app := NewApp("test")
	runtime := app.DefaultRuntime([]string{"hello"})

	auto, ok := runtime.(AutoRuntime)
	if !ok {
		t.Fatalf("expected AutoRuntime, got %T", runtime)
	}
	if len(auto.Args) != 1 || auto.Args[0] != "hello" {
		t.Fatalf("unexpected args in default runtime: %+v", auto.Args)
	}
	if auto.In == nil || auto.Out == nil || auto.Err == nil {
		t.Fatalf("expected standard streams in default runtime, got in=%v out=%v err=%v", auto.In, auto.Out, auto.Err)
	}
}

func TestRunDefaultUsesAutoRuntime(t *testing.T) {
	app := NewApp("test")
	app.EnableREPL()

	var ran bool
	app.Commands = []*Command{
		{
			Name: "hello",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				ran = true
				return nil
			},
		},
	}

	if err := app.RunDefault(context.Background(), []string{"hello"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Fatal("expected RunDefault to execute through the default runtime")
	}
}

func TestRunAutoAliasesDefaultRuntime(t *testing.T) {
	app := NewApp("test")

	var ran bool
	app.Commands = []*Command{
		{
			Name: "hello",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				ran = true
				return nil
			},
		},
	}

	if err := app.RunAuto(context.Background(), []string{"hello"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Fatal("expected RunAuto to execute through the auto runtime")
	}
}

func TestAppMainReportsErrorAndReturnsExitStatus(t *testing.T) {
	app := NewApp("test")
	var stderr bytes.Buffer
	app.Err = &stderr

	code := app.Main(context.Background(), CLIRuntime{Args: []string{"missing"}})
	if code != 2 {
		t.Fatalf("expected exit status 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), `unknown command "missing"`) {
		t.Fatalf("expected command error in stderr, got %q", stderr.String())
	}
}

func TestMainAutoAliasesDefaultRuntime(t *testing.T) {
	app := NewApp("test")
	app.SetRootCommand(&Command{
		Name: "test",
		Run: func(ctx context.Context, cmd *Command, args []string) error {
			return nil
		},
	})

	if code := app.MainAuto(context.Background(), nil); code != 0 {
		t.Fatalf("expected zero exit code, got %d", code)
	}
}

func TestMainWithContextUsesExitProcess(t *testing.T) {
	app := NewApp("test")
	app.SetRootCommand(&Command{
		Name: "test",
		Run: func(ctx context.Context, cmd *Command, args []string) error {
			return nil
		},
	})

	oldExit := exitProcess
	defer func() { exitProcess = oldExit }()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"test"}

	called := false
	code := -1
	exitProcess = func(got int) {
		called = true
		code = got
	}

	MainWithContext(context.Background(), app)

	if !called {
		t.Fatal("expected MainWithContext to invoke exitProcess")
	}
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d", code)
	}
}

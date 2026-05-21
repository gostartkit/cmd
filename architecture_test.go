package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSharedCommandTreeRoutesCLIAndREPL(t *testing.T) {
	app := NewApp("test")

	var seen []*Command
	var seenArgs [][]string

	leaf := &Command{
		Name: "users",
		Run: func(ctx context.Context, cmd *Command, args []string) error {
			seen = append(seen, cmd)
			seenArgs = append(seenArgs, append([]string(nil), args...))
			return nil
		},
	}
	app.Commands = []*Command{
		{
			Name: "admin",
			SubCommands: []*Command{
				leaf,
			},
		},
	}

	registry := app.registry()
	if got := registry.Lookup(nil, "admin"); got != app.Commands[0] {
		t.Fatalf("expected registry top-level lookup to return admin command, got %v", got)
	}
	if got := registry.ByPath["admin users"]; got != leaf {
		t.Fatalf("expected registry path lookup to return leaf command, got %v", got)
	}

	if err := app.Run(context.Background(), []string{"admin", "users", "sam"}); err != nil {
		t.Fatalf("cli run failed: %v", err)
	}
	if err := app.RunLine(context.Background(), `admin users "sam lee"`); err != nil {
		t.Fatalf("repl run failed: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("expected command to run twice, got %d", len(seen))
	}
	if seen[0] != leaf || seen[1] != leaf {
		t.Fatalf("expected cli and repl to route to shared leaf pointer, got %p and %p want %p", seen[0], seen[1], leaf)
	}
	if !slices.Equal(seenArgs[0], []string{"sam"}) {
		t.Fatalf("unexpected cli args: %v", seenArgs[0])
	}
	if !slices.Equal(seenArgs[1], []string{"sam lee"}) {
		t.Fatalf("unexpected repl args: %v", seenArgs[1])
	}
}

func TestCompletionSharesRegistryTree(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "deploy",
			SubCommands: []*Command{
				{Name: "status", Short: "show status"},
			},
		},
	}

	registry := app.registry()
	if got := registry.ByPath["deploy status"]; got == nil || got.Name != "status" {
		t.Fatalf("expected registry path index for deploy status, got %v", got)
	}

	root := app.CompleteLine("dep", len("dep"))
	if !slices.Contains(root, "deploy") {
		t.Fatalf("expected root completion to include deploy, got %v", root)
	}

	sub := app.CompleteLine("deploy st", len("deploy st"))
	if !slices.Contains(sub, "status") {
		t.Fatalf("expected subcommand completion to include status, got %v", sub)
	}
}

func TestRegistryBuiltinsRemainCompatible(t *testing.T) {
	t.Run("builtin spec still works", func(t *testing.T) {
		app := NewApp("test")
		var output bytes.Buffer
		app.Out = &output
		app.Err = &output

		if err := app.Run(context.Background(), []string{"spec"}); err != nil {
			t.Fatalf("spec builtin failed: %v", err)
		}
		if !strings.Contains(output.String(), `"name": "test"`) {
			t.Fatalf("expected spec output, got %q", output.String())
		}
	})

	t.Run("top-level command can still shadow builtin", func(t *testing.T) {
		app := NewApp("test")
		var ran bool
		app.Commands = []*Command{
			{
				Name: "spec",
				Run: func(ctx context.Context, cmd *Command, args []string) error {
					ran = true
					return nil
				},
			},
		}

		if err := app.Run(context.Background(), []string{"spec"}); err != nil {
			t.Fatalf("shadowed spec command failed: %v", err)
		}
		if !ran {
			t.Fatal("expected custom spec command to shadow builtin")
		}
	})
}

func TestResolverPreservesFlagsPositionalsConfigAndEnv(t *testing.T) {
	app := NewApp("test")
	app.EnableConfigSupport()

	var verbose bool
	var name string
	var seen []string

	app.SetFlags = func(f *FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
	}
	app.Commands = []*Command{
		{
			Name: "greet",
			Positionals: []PositionalArg{
				{Name: "target", Required: true},
			},
			SetFlags: func(f *FlagSet) {
				f.StringVar(&name, "name", "guest", "name value", "n")
				f.BindEnv("name", "APP_NAME")
				f.BindConfig("name", "svc.name")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				seen = append(seen, strings.Join([]string{args[0], name, boolString(verbose)}, "|"))
				return nil
			},
		},
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"svc":{"name":"from-config"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("APP_NAME", "from-env")

	if err := app.Run(context.Background(), []string{"--config", configPath, "greet", "team"}); err != nil {
		t.Fatalf("cli run failed: %v", err)
	}
	if err := app.RunLine(context.Background(), "--config "+configPath+" --verbose greet squad --name from-cli"); err != nil {
		t.Fatalf("repl run failed: %v", err)
	}

	want := []string{
		"team|from-env|false",
		"squad|from-cli|true",
	}
	if !slices.Equal(seen, want) {
		t.Fatalf("unexpected execution results: got %v want %v", seen, want)
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

package cmd

import (
	"reflect"
	"slices"
	"testing"
)

func TestAppSpecForSurfaceOverrides(t *testing.T) {
	t.Parallel()

	createUser := &Command{
		Name:      "user",
		ID:        "command.create.user",
		HandlerID: "handler.create.user",
		UsageLine: "test create user <name> [flags]",
		Positionals: []PositionalArg{{
			Name:          "name",
			Usage:         "MySQL username",
			Required:      true,
			Kind:          "user",
			CompletionKey: "user",
			Surfaces: map[Surface]PositionalSurface{
				SurfaceREPL: {Required: boolRef(false)},
			},
		}},
		Surfaces: map[Surface]CommandSurface{
			SurfaceREPL: {UsageLine: "test create user [name] [flags]"},
		},
		SetFlags: func(f *FlagSet) {
			f.Func("input", "template input in key=value form", func(string) error { return nil }, "")
			f.SetID("input", "flag.create.user.input")
			f.SetKind("input", "template_input")
			f.MarkRepeatable("input")
			f.SetCompletionKey("input", "template-input")
			f.SetSurface("input", SurfaceREPL, FlagSurface{
				Usage:         "interactive template input",
				CompletionKey: "repl-template-input",
			})
		},
	}

	createDatabase := &Command{
		Name:      "database",
		UsageLine: "test create database <name>",
		Positionals: []PositionalArg{{
			Name:     "name",
			Usage:    "database name",
			Required: true,
			Kind:     "database",
		}},
		Surfaces: map[Surface]CommandSurface{
			SurfaceREPL: {
				UsageLine:          "test create database",
				ReplacePositionals: true,
			},
		},
	}

	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "create",
			SubCommands: []*Command{
				createUser,
				createDatabase,
			},
		},
	}

	cliSpec := app.SpecFor(SurfaceCLI)
	replSpec := app.SpecFor(SurfaceREPL)

	if cliSpec.Surface != "cli" || replSpec.Surface != "repl" {
		t.Fatalf("unexpected surfaces cli=%q repl=%q", cliSpec.Surface, replSpec.Surface)
	}
	if !slices.Equal(replSpec.AvailableSurfaces, []string{"repl"}) {
		t.Fatalf("unexpected available surfaces: %+v", replSpec.AvailableSurfaces)
	}
	if !replSpec.Capabilities.SurfaceOverrides || !replSpec.Capabilities.StableIDs || !replSpec.Capabilities.SemanticKinds || !replSpec.Capabilities.RepeatableFlags || !replSpec.Capabilities.CompletionKeys {
		t.Fatalf("expected rich spec capabilities, got %+v", replSpec.Capabilities)
	}

	createCLI := cliSpec.Commands[0]
	createREPL := replSpec.Commands[0]
	userCLI := createCLI.SubCommands[0]
	userREPL := createREPL.SubCommands[0]
	databaseCLI := createCLI.SubCommands[1]
	databaseREPL := createREPL.SubCommands[1]

	if !reflect.DeepEqual(userCLI.Path, []string{"create", "user"}) {
		t.Fatalf("unexpected command path: %+v", userCLI.Path)
	}
	if userCLI.ID != "command.create.user" || userCLI.HandlerID != "handler.create.user" {
		t.Fatalf("unexpected stable ids: %+v", userCLI)
	}

	if userCLI.UsageLine != "test create user <name> [flags]" {
		t.Fatalf("unexpected cli usage: %q", userCLI.UsageLine)
	}
	if userREPL.UsageLine != "test create user [name] [flags]" {
		t.Fatalf("unexpected repl usage: %q", userREPL.UsageLine)
	}

	if len(userCLI.Positionals) != 1 || !userCLI.Positionals[0].Required || userCLI.Positionals[0].Kind != "user" || userCLI.Positionals[0].CompletionKey != "user" {
		t.Fatalf("unexpected cli positional spec: %+v", userCLI.Positionals)
	}
	if len(userREPL.Positionals) != 1 || userREPL.Positionals[0].Required || userREPL.Positionals[0].Kind != "user" || userREPL.Positionals[0].CompletionKey != "user" {
		t.Fatalf("unexpected repl positional spec: %+v", userREPL.Positionals)
	}

	if len(databaseCLI.Positionals) != 1 || databaseCLI.Positionals[0].Name != "name" {
		t.Fatalf("unexpected cli database positional spec: %+v", databaseCLI.Positionals)
	}
	if len(databaseREPL.Positionals) != 0 {
		t.Fatalf("expected repl database spec to remove positionals, got %+v", databaseREPL.Positionals)
	}

	if len(userCLI.Flags) != 1 || userCLI.Flags[0].ID != "flag.create.user.input" || userCLI.Flags[0].Kind != "template_input" || !userCLI.Flags[0].Repeatable || userCLI.Flags[0].CompletionKey != "template-input" {
		t.Fatalf("unexpected cli flag spec: %+v", userCLI.Flags)
	}
	if len(userREPL.Flags) != 1 || userREPL.Flags[0].Usage != "interactive template input" || userREPL.Flags[0].CompletionKey != "repl-template-input" {
		t.Fatalf("unexpected repl flag spec: %+v", userREPL.Flags)
	}
}

func TestAppSpecIncludesRichMetadata(t *testing.T) {
	t.Parallel()

	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name:      "exec",
			UsageLine: "test exec <target> [flags]",
			Positionals: []PositionalArg{{
				Name:          "target",
				Usage:         "execution target",
				Kind:          "operation",
				CompletionKey: "operation",
				Required:      true,
			}},
			SetFlags: func(f *FlagSet) {
				f.Func("input", "template input", func(string) error { return nil }, "")
				f.SetKind("input", "template_input")
				f.MarkRepeatable("input")
				f.SetCompletionKey("input", "template-input")
			},
		},
	}

	spec := app.Spec()
	if len(spec.Commands) != 1 {
		t.Fatalf("expected one command, got %+v", spec.Commands)
	}

	command := spec.Commands[0]
	if command.ID != "exec" || command.HandlerID != "exec" || !reflect.DeepEqual(command.Path, []string{"exec"}) {
		t.Fatalf("unexpected command identity metadata: %+v", command)
	}
	if len(command.Positionals) != 1 || command.Positionals[0].ID != "exec#0" || command.Positionals[0].Kind != "operation" || command.Positionals[0].CompletionKey != "operation" {
		t.Fatalf("unexpected positional metadata: %+v", command.Positionals)
	}
	if len(command.Flags) != 1 || command.Flags[0].ID != "exec#input" || command.Flags[0].Kind != "template_input" || !command.Flags[0].Repeatable || command.Flags[0].CompletionKey != "template-input" {
		t.Fatalf("unexpected flag metadata: %+v", command.Flags)
	}
}

func TestAppSpecDefaultSurfaceCompatibility(t *testing.T) {
	t.Parallel()

	app := NewApp("test")
	app.Commands = []*Command{{Name: "version", UsageLine: "test version"}}

	legacy := app.Spec()
	explicit := app.SpecFor("")

	if legacy.Surface != "" || explicit.Surface != "" {
		t.Fatalf("expected default spec to omit surface, got legacy=%q explicit=%q", legacy.Surface, explicit.Surface)
	}
	if !reflect.DeepEqual(legacy, explicit) {
		t.Fatalf("expected Spec() and SpecFor(\"\") to match\nlegacy=%+v\nexplicit=%+v", legacy, explicit)
	}
}

func boolRef(value bool) *bool {
	return &value
}

package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

type csvValue []string

func (c *csvValue) String() string {
	return strings.Join(*c, ",")
}

func (c *csvValue) Set(s string) error {
	*c = strings.Split(s, ",")
	return nil
}

func TestFlagSetCachedDefinitionLookupVisitAll(t *testing.T) {
	var (
		verbose bool
		name    string
		count   int
		timeout time.Duration
	)

	cache := buildCachedFlagDefinition("cached", func(f *FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
		f.StringVar(&name, "name", "guest", "name value", "n")
		f.IntVar(&count, "count", 1, "count value", "c")
		f.DurationVar(&timeout, "timeout", time.Second, "timeout value", "t")
		f.BindEnv("name", "APP_NAME")
		f.BindConfig("count", "svc.count")
		f.SetEnum("name", "guest", "sam")
		f.MarkRequired("name")
		f.MarkHidden("timeout")
		f.MarkDeprecated("count", "use --replicas")
		f.SetCompletion("name", func(ctx CompletionContext) []string {
			return []string{"sam", "sara"}
		})
	})
	if !cache.Cacheable {
		t.Fatal("expected cacheable definition")
	}

	flagSet, ok := instantiateCachedFlagDefinition(cache, "cached", ioDiscard())
	if !ok {
		t.Fatal("expected cached instantiation")
	}

	nameFlag, ok := flagSet.Lookup("name")
	if !ok {
		t.Fatal("expected name lookup")
	}
	if nameFlag.Shorthand != "n" || !nameFlag.Required || nameFlag.Hidden {
		t.Fatalf("unexpected name metadata: %+v", nameFlag)
	}

	if err := os.Setenv("APP_NAME", "env-name"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	defer os.Unsetenv("APP_NAME")

	if err := flagSet.ApplyEnv(); err != nil {
		t.Fatalf("apply env: %v", err)
	}
	if err := flagSet.ApplyConfig(map[string]any{"svc": map[string]any{"count": 9.0}}); err != nil {
		t.Fatalf("apply config: %v", err)
	}
	if err := flagSet.Parse([]string{"--verbose", "--name", "sam"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := flagSet.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	if !verbose || name != "sam" || count != 9 || timeout != time.Second {
		t.Fatalf("unexpected values verbose=%v name=%q count=%d timeout=%s", verbose, name, count, timeout)
	}
	if !flagSet.IsSet("verbose") || !flagSet.IsSet("name") || !flagSet.IsSet("count") {
		t.Fatalf("expected set flags to be tracked")
	}

	var visitedAll []string
	flagSet.VisitAll(func(flag *Flag) {
		visitedAll = append(visitedAll, flag.Name)
	})
	if !slices.Equal(visitedAll, []string{"count", "name", "timeout", "verbose"}) {
		t.Fatalf("unexpected visit all order: %v", visitedAll)
	}

	var visitedSet []string
	flagSet.Visit(func(flag *Flag) {
		visitedSet = append(visitedSet, flag.Name)
	})
	if !slices.Equal(visitedSet, []string{"count", "name", "verbose"}) {
		t.Fatalf("unexpected visit set order: %v", visitedSet)
	}
}

func TestRunWithCachedFlagsDoesNotLeakState(t *testing.T) {
	app := NewApp("test")
	app.Out = ioDiscard()
	app.Err = ioDiscard()

	var (
		verbose bool
		profile string
		env     string
		count   int
	)

	app.SetFlags = func(f *FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
		f.StringVar(&profile, "profile", "default", "profile name", "p")
	}
	app.Commands = []*Command{
		{
			Name: "deploy",
			SetFlags: func(f *FlagSet) {
				f.StringVar(&env, "env", "dev", "target env", "e")
				f.IntVar(&count, "count", 1, "instance count", "c")
				f.SetEnum("env", "dev", "prod")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error { return nil },
		},
	}

	root := app.rootCommand()
	if def, ok := app.rootFlagDefinition(root); !ok || def == nil || !def.Cacheable {
		t.Fatal("expected cacheable root flag definition")
	}
	if def, ok := app.Commands[0].localFlagDefinition(); !ok || def == nil || !def.Cacheable {
		t.Fatal("expected cacheable local flag definition")
	}

	if err := app.Run(context.Background(), []string{"--verbose", "--profile", "prod", "deploy", "--env", "prod", "--count", "3"}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if !verbose || profile != "prod" || env != "prod" || count != 3 {
		t.Fatalf("unexpected first run values verbose=%v profile=%q env=%q count=%d", verbose, profile, env, count)
	}

	if err := app.Run(context.Background(), []string{"deploy"}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if verbose || profile != "default" || env != "dev" || count != 1 {
		t.Fatalf("expected reset defaults verbose=%v profile=%q env=%q count=%d", verbose, profile, env, count)
	}
}

func TestREPLWithCachedFlagsDoesNotLeakState(t *testing.T) {
	app := NewApp("test")
	var (
		verbose bool
		name    string
	)

	app.SetFlags = func(f *FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
	}
	app.Commands = []*Command{
		{
			Name: "hello",
			SetFlags: func(f *FlagSet) {
				f.StringVar(&name, "name", "guest", "person name", "n")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error { return nil },
		},
	}

	repl := &REPL{
		App: app,
		In:  strings.NewReader("--verbose hello --name sam\nhello\n.exit\n"),
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
	}
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("repl run: %v", err)
	}
	if verbose || name != "guest" {
		t.Fatalf("expected repl state isolation verbose=%v name=%q", verbose, name)
	}
}

func TestCustomVarFallsBackAndStaysIsolated(t *testing.T) {
	app := NewApp("test")
	app.Out = ioDiscard()
	app.Err = ioDiscard()

	var values csvValue
	app.Commands = []*Command{
		{
			Name: "list",
			SetFlags: func(f *FlagSet) {
				f.Var(&values, "values", "csv values", "v")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error { return nil },
		},
	}

	if def, ok := app.Commands[0].localFlagDefinition(); !ok || def == nil || !def.Cacheable {
		t.Fatal("expected custom Var command flags to use resettable cached path")
	}

	if err := app.Run(context.Background(), []string{"list", "--values", "a,b"}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if !slices.Equal([]string(values), []string{"a", "b"}) {
		t.Fatalf("unexpected first custom values: %v", values)
	}

	if err := app.Run(context.Background(), []string{"list"}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("expected fallback run to reset custom values, got %v", values)
	}
}

func TestCompleteDetailedFlagsWithCachedDefinitions(t *testing.T) {
	app := benchmarkFlagApp()
	results := app.CompleteLineDetailed("deploy --e", len("deploy --e"))
	if len(results) == 0 {
		t.Fatal("expected completion results")
	}

	found := false
	for _, result := range results {
		if result.Value == "--env" {
			found = true
			if result.Kind != completionKindFlag {
				t.Fatalf("expected flag kind, got %q", result.Kind)
			}
			if result.Description == "" {
				t.Fatal("expected flag description")
			}
		}
	}
	if !found {
		t.Fatalf("expected --env completion, got %+v", results)
	}
}

func TestLookupFlagMutationDoesNotLeakEnum(t *testing.T) {
	app, cmd := runtimeMutationTestApp()

	flagSet := runtimeMutationFlagSet(t, app, cmd)
	flag, ok := flagSet.Lookup("name")
	if !ok {
		t.Fatal("expected name flag")
	}
	flag.Enum[0] = "hacked"
	flag.Enum = append(flag.Enum, "extra")

	next := runtimeMutationFlagSet(t, app, cmd)
	nextFlag, ok := next.Lookup("name")
	if !ok {
		t.Fatal("expected name flag on second lookup")
	}
	if !slices.Equal(nextFlag.Enum, []string{"sam", "sara"}) {
		t.Fatalf("expected enum isolation, got %v", nextFlag.Enum)
	}
	if got := app.CompleteLine("greet --name s", len("greet --name s")); !slices.Equal(got, []string{"sam", "sara"}) {
		t.Fatalf("expected completion to use original enum, got %v", got)
	}
}

func TestLookupFlagMutationDoesNotLeakEnvVars(t *testing.T) {
	app, cmd := runtimeMutationTestApp()

	flagSet := runtimeMutationFlagSet(t, app, cmd)
	flag, ok := flagSet.Lookup("name")
	if !ok {
		t.Fatal("expected name flag")
	}
	flag.EnvVars[0] = "HACKED_ENV"
	flag.EnvVars = append(flag.EnvVars, "OTHER_ENV")

	next := runtimeMutationFlagSet(t, app, cmd)
	nextFlag, ok := next.Lookup("name")
	if !ok {
		t.Fatal("expected name flag on second lookup")
	}
	if !slices.Equal(nextFlag.EnvVars, []string{"APP_NAME"}) {
		t.Fatalf("expected env var isolation, got %v", nextFlag.EnvVars)
	}
}

func TestLookupFlagMutationDoesNotLeakConfigKeys(t *testing.T) {
	app, cmd := runtimeMutationTestApp()

	flagSet := runtimeMutationFlagSet(t, app, cmd)
	flag, ok := flagSet.Lookup("name")
	if !ok {
		t.Fatal("expected name flag")
	}
	flag.ConfigKeys[0] = "hacked.key"
	flag.ConfigKeys = append(flag.ConfigKeys, "other.key")

	next := runtimeMutationFlagSet(t, app, cmd)
	nextFlag, ok := next.Lookup("name")
	if !ok {
		t.Fatal("expected name flag on second lookup")
	}
	if !slices.Equal(nextFlag.ConfigKeys, []string{"profile.name"}) {
		t.Fatalf("expected config key isolation, got %v", nextFlag.ConfigKeys)
	}
}

func TestLookupFlagMutationDoesNotLeakExtensions(t *testing.T) {
	app, cmd := runtimeMutationTestApp()

	flagSet := runtimeMutationFlagSet(t, app, cmd)
	flag, ok := flagSet.Lookup("name")
	if !ok {
		t.Fatal("expected name flag")
	}
	flag.Extensions["x-ui-control"] = "mutated"
	flag.Extensions["x-added"] = "yes"

	next := runtimeMutationFlagSet(t, app, cmd)
	nextFlag, ok := next.Lookup("name")
	if !ok {
		t.Fatal("expected name flag on second lookup")
	}
	if nextFlag.Extensions["x-ui-control"] != "picker" {
		t.Fatalf("expected extension isolation, got %v", nextFlag.Extensions["x-ui-control"])
	}
	if _, exists := nextFlag.Extensions["x-added"]; exists {
		t.Fatalf("expected extension map isolation, got %+v", nextFlag.Extensions)
	}
}

func TestVisitAllFlagMutationDoesNotLeak(t *testing.T) {
	app, cmd := runtimeMutationTestApp()

	flagSet := runtimeMutationFlagSet(t, app, cmd)
	flagSet.VisitAll(func(flag *Flag) {
		if flag.Name != "name" {
			return
		}
		flag.Enum[0] = "visit-mutated"
		flag.EnvVars[0] = "VISIT_ENV"
		flag.ConfigKeys[0] = "visit.key"
		flag.Extensions["x-ui-control"] = "visit-mutated"
	})

	next := runtimeMutationFlagSet(t, app, cmd)
	nextFlag, ok := next.Lookup("name")
	if !ok {
		t.Fatal("expected name flag on second lookup")
	}
	if !slices.Equal(nextFlag.Enum, []string{"sam", "sara"}) {
		t.Fatalf("expected enum isolation after VisitAll, got %v", nextFlag.Enum)
	}
	if !slices.Equal(nextFlag.EnvVars, []string{"APP_NAME"}) {
		t.Fatalf("expected env var isolation after VisitAll, got %v", nextFlag.EnvVars)
	}
	if !slices.Equal(nextFlag.ConfigKeys, []string{"profile.name"}) {
		t.Fatalf("expected config key isolation after VisitAll, got %v", nextFlag.ConfigKeys)
	}
	if nextFlag.Extensions["x-ui-control"] != "picker" {
		t.Fatalf("expected extension isolation after VisitAll, got %+v", nextFlag.Extensions)
	}
}

func TestRepeatedRunAfterLookupMutation(t *testing.T) {
	app, cmd := runtimeMutationTestApp()
	app.Out = io.Discard
	app.Err = io.Discard

	if err := os.Setenv("APP_NAME", "env-sam"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	defer os.Unsetenv("APP_NAME")

	if err := app.Run(context.Background(), []string{"greet"}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if cmd.flag == nil {
		t.Fatal("expected command flag set after first run")
	}
	flag, ok := cmd.flag.Lookup("name")
	if !ok {
		t.Fatal("expected name flag after first run")
	}
	flag.Enum[0] = "mutated"
	flag.EnvVars[0] = "BROKEN_ENV"
	flag.ConfigKeys[0] = "broken.key"
	flag.Extensions["x-ui-control"] = "broken"

	if err := app.Run(context.Background(), []string{"greet"}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if cmd.flag == nil {
		t.Fatal("expected command flag set after second run")
	}
	nextFlag, ok := cmd.flag.Lookup("name")
	if !ok {
		t.Fatal("expected name flag after second run")
	}
	if !slices.Equal(nextFlag.Enum, []string{"sam", "sara"}) {
		t.Fatalf("expected enum reset after run, got %v", nextFlag.Enum)
	}
	if !slices.Equal(nextFlag.EnvVars, []string{"APP_NAME"}) {
		t.Fatalf("expected env vars reset after run, got %v", nextFlag.EnvVars)
	}
	if !slices.Equal(nextFlag.ConfigKeys, []string{"profile.name"}) {
		t.Fatalf("expected config keys reset after run, got %v", nextFlag.ConfigKeys)
	}
	if nextFlag.Extensions["x-ui-control"] != "picker" {
		t.Fatalf("expected extensions reset after run, got %+v", nextFlag.Extensions)
	}
	var helpOut bytes.Buffer
	app.Out = &helpOut
	app.Err = &helpOut
	if err := app.help([]string{"greet"}); err != nil {
		t.Fatalf("help after mutation: %v", err)
	}
	usageText := helpOut.String()
	if !strings.Contains(usageText, "choices: sam, sara") {
		t.Fatalf("expected help to keep original enum, got %q", usageText)
	}
	if !strings.Contains(usageText, "env: APP_NAME") {
		t.Fatalf("expected help to keep original env binding, got %q", usageText)
	}
	if !strings.Contains(usageText, "config: profile.name") {
		t.Fatalf("expected help to keep original config binding, got %q", usageText)
	}
	spec := app.Spec()
	if len(spec.Commands) == 0 || len(spec.Commands[0].Flags) == 0 {
		t.Fatal("expected command flags in spec")
	}
	var specFlag FlagSpec
	for _, candidate := range spec.Commands[0].Flags {
		if candidate.Name == "name" {
			specFlag = candidate
			break
		}
	}
	if !slices.Equal(specFlag.Enum, []string{"sam", "sara"}) {
		t.Fatalf("expected spec enum to remain original, got %v", specFlag.Enum)
	}
}

func runtimeMutationTestApp() (*App, *Command) {
	app := NewApp("test")
	var name string
	command := &Command{
		Name: "greet",
		SetFlags: func(f *FlagSet) {
			f.StringVar(&name, "name", "", "person name", "n")
			f.SetEnum("name", "sam", "sara")
			f.BindEnv("name", "APP_NAME")
			f.BindConfig("name", "profile.name")
			_ = f.SetExtension("name", "x-ui-control", "picker")
		},
		Run: func(ctx context.Context, cmd *Command, args []string) error {
			return nil
		},
	}
	app.Commands = []*Command{command}
	return app, command
}

func runtimeMutationFlagSet(t *testing.T, app *App, cmd *Command) *FlagSet {
	t.Helper()
	root := app.rootCommand()
	rootFlags := app.newRootFlagSetFor(root, io.Discard)
	return app.newCommandFlagSetFor(root, rootFlags, cmd, io.Discard)
}

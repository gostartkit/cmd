package cmd

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

type customSliceValue []string

func (c *customSliceValue) String() string {
	return strings.Join(*c, ",")
}

func (c *customSliceValue) Set(s string) error {
	*c = strings.Split(s, ",")
	return nil
}

func TestCachedFlagDefinitionInstantiateSupportedTypes(t *testing.T) {
	var (
		verbose bool
		name    string
		count   int
		timeout time.Duration
	)

	cache := buildCachedFlagDefinition("test", func(f *FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
		f.StringVar(&name, "name", "guest", "name value", "n")
		f.IntVar(&count, "count", 1, "count value", "c")
		f.DurationVar(&timeout, "timeout", time.Second, "timeout value", "t")
		f.BindEnv("name", "APP_NAME")
		f.BindConfig("count", "count")
		f.SetEnum("name", "guest", "sam")
		f.MarkRequired("name")
		f.MarkHidden("timeout")
		f.MarkDeprecated("count", "use --replicas")
		f.SetExample("name", "sam")
		f.SetCategory("name", "General")
		f.SetCompletion("name", func(ctx CompletionContext) []string {
			return []string{"sam", "sara"}
		})
	})

	if !cache.cacheable {
		t.Fatal("expected builtin flag definition to be cacheable")
	}

	flagSet, ok := instantiateCachedFlagDefinition(cache, "run1", ioDiscard())
	if !ok {
		t.Fatal("expected cached definition instantiation to succeed")
	}

	nameFlag, exists := flagSet.Lookup("name")
	if !exists {
		t.Fatal("expected name flag")
	}
	if !nameFlag.Required || nameFlag.Hidden || nameFlag.Deprecated != "" || nameFlag.Category != "General" {
		t.Fatalf("unexpected name metadata: %+v", nameFlag)
	}
	if nameFlag.Completion == nil || !slices.Equal(nameFlag.Completion(CompletionContext{}), []string{"sam", "sara"}) {
		t.Fatalf("expected completion to be preserved, got %+v", nameFlag)
	}

	timeoutFlag, exists := flagSet.Lookup("timeout")
	if !exists || !timeoutFlag.Hidden {
		t.Fatalf("expected hidden timeout flag, got %+v", timeoutFlag)
	}
	countFlag, exists := flagSet.Lookup("count")
	if !exists || countFlag.Deprecated != "use --replicas" {
		t.Fatalf("expected deprecated count flag, got %+v", countFlag)
	}
	if !slices.Equal(nameFlag.Enum, []string{"guest", "sam"}) || nameFlag.Example != "sam" {
		t.Fatalf("expected enum/example metadata, got %+v", nameFlag)
	}

	if err := flagSet.Parse([]string{"--verbose", "--name", "sam", "--count", "7", "--timeout", "2s"}); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !verbose || name != "sam" || count != 7 || timeout != 2*time.Second {
		t.Fatalf("expected parsed values, got verbose=%v name=%q count=%d timeout=%s", verbose, name, count, timeout)
	}

	second, ok := instantiateCachedFlagDefinition(cache, "run2", ioDiscard())
	if !ok {
		t.Fatal("expected second instantiation to succeed")
	}
	if verbose || name != "guest" || count != 1 || timeout != time.Second {
		t.Fatalf("expected defaults reset on second instantiation, got verbose=%v name=%q count=%d timeout=%s", verbose, name, count, timeout)
	}
	if err := second.Parse([]string{"-n", "guest"}); err != nil {
		t.Fatalf("second parse failed: %v", err)
	}
}

func TestCachedFlagDefinitionApplyEnvAndConfig(t *testing.T) {
	var (
		name  string
		count int
	)

	cache := buildCachedFlagDefinition("test", func(f *FlagSet) {
		f.StringVar(&name, "name", "guest", "name value", "n")
		f.IntVar(&count, "count", 1, "count value", "c")
		f.BindEnv("name", "APP_NAME")
		f.BindConfig("count", "service.count")
	})

	flagSet, ok := instantiateCachedFlagDefinition(cache, "envcfg", ioDiscard())
	if !ok {
		t.Fatal("expected cached definition instantiation to succeed")
	}

	if err := os.Setenv("APP_NAME", "env-name"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	defer os.Unsetenv("APP_NAME")

	if err := flagSet.ApplyEnv(); err != nil {
		t.Fatalf("apply env: %v", err)
	}
	if err := flagSet.ApplyConfig(map[string]any{"service": map[string]any{"count": 9.0}}); err != nil {
		t.Fatalf("apply config: %v", err)
	}

	if name != "env-name" || count != 9 {
		t.Fatalf("expected env/config values, got name=%q count=%d", name, count)
	}
}

func TestCachedFlagDefinitionWarnDeprecated(t *testing.T) {
	var count int

	cache := buildCachedFlagDefinition("test", func(f *FlagSet) {
		f.IntVar(&count, "count", 1, "count value", "c")
		f.MarkDeprecated("count", "use --replicas")
	})

	var out bytes.Buffer
	flagSet, ok := instantiateCachedFlagDefinition(cache, "warn", &out)
	if !ok {
		t.Fatal("expected cached definition instantiation to succeed")
	}

	if err := flagSet.Parse([]string{"--count", "3"}); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	flagSet.WarnDeprecated()
	if !strings.Contains(out.String(), "Warning: flag --count is deprecated: use --replicas") {
		t.Fatalf("expected deprecated warning, got %q", out.String())
	}
}

func TestCachedFlagDefinitionCustomVarFallback(t *testing.T) {
	var values customSliceValue

	cache := buildCachedFlagDefinition("test", func(f *FlagSet) {
		f.Var(&values, "values", "csv values", "v")
	})

	if cache.cacheable {
		t.Fatal("expected custom Var definition to be non-cacheable")
	}
	if _, ok := instantiateCachedFlagDefinition(cache, "custom", ioDiscard()); ok {
		t.Fatal("expected cached instantiation to fail for custom Var")
	}
}

func TestCachedFlagDefinitionSupportsRepeatedRunLikeReset(t *testing.T) {
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
			Name: "sayhi",
			SetFlags: func(f *FlagSet) {
				f.StringVar(&name, "name", "guest", "name value", "n")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	if err := app.RunLine(context.Background(), "--verbose sayhi --name sam"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := app.RunLine(context.Background(), "sayhi"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if verbose || name != "guest" {
		t.Fatalf("expected main path isolation to remain intact, got verbose=%v name=%q", verbose, name)
	}
}

func ioDiscard() *bytes.Buffer {
	return &bytes.Buffer{}
}

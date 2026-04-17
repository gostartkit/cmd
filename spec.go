package cmd

import (
	"encoding/json"
	"fmt"
	"io"
)

type AppSpec struct {
	Name        string        `json:"name"`
	Short       string        `json:"short"`
	GlobalFlags []FlagSpec    `json:"global_flags,omitempty"`
	Commands    []CommandSpec `json:"commands"`
}

type CommandSpec struct {
	Name        string           `json:"name"`
	Aliases     []string         `json:"aliases,omitempty"`
	UsageLine   string           `json:"usage_line,omitempty"`
	Short       string           `json:"short,omitempty"`
	Long        string           `json:"long,omitempty"`
	Group       string           `json:"group,omitempty"`
	Deprecated  string           `json:"deprecated,omitempty"`
	Hidden      bool             `json:"hidden,omitempty"`
	Examples    []string         `json:"examples,omitempty"`
	Positionals []PositionalSpec `json:"positionals,omitempty"`
	Flags       []FlagSpec       `json:"flags,omitempty"`
	SubCommands []CommandSpec    `json:"subcommands,omitempty"`
}

type PositionalSpec struct {
	Name     string   `json:"name"`
	Usage    string   `json:"usage,omitempty"`
	Required bool     `json:"required,omitempty"`
	Variadic bool     `json:"variadic,omitempty"`
	Enum     []string `json:"enum,omitempty"`
	Example  string   `json:"example,omitempty"`
}

type FlagSpec struct {
	Name       string   `json:"name"`
	Shorthand  string   `json:"shorthand,omitempty"`
	Type       string   `json:"type,omitempty"`
	Usage      string   `json:"usage,omitempty"`
	Default    string   `json:"default,omitempty"`
	Category   string   `json:"category,omitempty"`
	EnvVars    []string `json:"env_vars,omitempty"`
	ConfigKeys []string `json:"config_keys,omitempty"`
	Enum       []string `json:"enum,omitempty"`
	Required   bool     `json:"required,omitempty"`
	Hidden     bool     `json:"hidden,omitempty"`
	Deprecated string   `json:"deprecated,omitempty"`
	Example    string   `json:"example,omitempty"`
}

func (a *App) runSpec(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: %s spec [json]", a.Name)
	}
	if len(args) == 1 && args[0] != "json" {
		return fmt.Errorf("usage: %s spec [json]", a.Name)
	}

	out := a.Out
	if out == nil {
		out = io.Discard
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(a.Spec())
}

func (a *App) Spec() AppSpec {
	spec := AppSpec{
		Name:     a.Name,
		Short:    a.Short,
		Commands: make([]CommandSpec, 0, len(a.Commands)),
	}
	if rootFlags := a.newRootFlagSet(); rootFlags != nil {
		spec.GlobalFlags = flagSetSpec(rootFlags)
	}
	for _, command := range a.Commands {
		spec.Commands = append(spec.Commands, a.commandSpec(command))
	}
	return spec
}

func (a *App) commandSpec(cmd *Command) CommandSpec {
	spec := CommandSpec{
		Name:        cmd.Name,
		Aliases:     append([]string(nil), cmd.Aliases...),
		UsageLine:   cmd.UsageLine,
		Short:       cmd.Short,
		Long:        cmd.Long,
		Group:       cmd.Group,
		Deprecated:  cmd.Deprecated,
		Hidden:      cmd.Hidden,
		Examples:    append([]string(nil), cmd.Examples...),
		Positionals: positionalSpecs(cmd.Positionals),
	}
	if localFlags := a.newCommandFlagSet(cmd); localFlags != nil {
		spec.Flags = flagSetSpec(localFlags)
	}
	if len(cmd.SubCommands) > 0 {
		spec.SubCommands = make([]CommandSpec, 0, len(cmd.SubCommands))
		for _, subCommand := range cmd.SubCommands {
			spec.SubCommands = append(spec.SubCommands, a.commandSpec(subCommand))
		}
	}
	return spec
}

func positionalSpecs(positionals []PositionalArg) []PositionalSpec {
	if len(positionals) == 0 {
		return nil
	}
	specs := make([]PositionalSpec, 0, len(positionals))
	for _, positional := range positionals {
		specs = append(specs, PositionalSpec{
			Name:     positional.Name,
			Usage:    positional.Usage,
			Required: positional.Required,
			Variadic: positional.Variadic,
			Enum:     append([]string(nil), positional.Enum...),
			Example:  positional.Example,
		})
	}
	return specs
}

func flagSetSpec(flagSet *FlagSet) []FlagSpec {
	flags := make([]FlagSpec, 0)
	flagSet.VisitAll(func(flag *Flag) {
		flags = append(flags, flagSpec(flag))
	})
	return flags
}

func flagSpec(flag *Flag) FlagSpec {
	flagType, usage := UnquoteUsage(flag)
	return FlagSpec{
		Name:       flag.Name,
		Shorthand:  flag.Shorthand,
		Type:       flagType,
		Usage:      usage,
		Default:    flag.DefValue,
		Category:   flag.Category,
		EnvVars:    append([]string(nil), flag.EnvVars...),
		ConfigKeys: append([]string(nil), flag.ConfigKeys...),
		Enum:       append([]string(nil), flag.Enum...),
		Required:   flag.Required,
		Hidden:     flag.Hidden,
		Deprecated: flag.Deprecated,
		Example:    flag.Example,
	}
}

package cmd

import (
	"fmt"
	"strings"
)

type BuiltinHandler func(app *App, args []string) error

// Registry is a read-only index over the shared command tree.
type Registry struct {
	Root     *Command
	ByPath   map[string]*Command
	Aliases  map[string]*Command
	Builtins map[string]BuiltinHandler

	commandsByParent map[*Command]map[string]*Command
	visibleBuiltins  []string
}

func (a *App) registry() *Registry {
	return newRegistry(a.rootCommand())
}

func newRegistry(root *Command) *Registry {
	registry := &Registry{
		Root:             root,
		ByPath:           make(map[string]*Command),
		Aliases:          make(map[string]*Command),
		Builtins:         make(map[string]BuiltinHandler),
		commandsByParent: make(map[*Command]map[string]*Command),
	}
	if root == nil {
		return registry
	}

	registry.indexCommands(nil, nil, root.SubCommands)
	registry.registerBuiltins(root.SubCommands)
	return registry
}

func (r *Registry) indexCommands(parent *Command, path []string, commands Commands) {
	if len(commands) == 0 {
		return
	}

	index := buildCommandLookup(commands)
	if len(index) > 0 {
		r.commandsByParent[parent] = index
	}

	for _, cmd := range commands {
		if cmd == nil || cmd.Name == "" {
			continue
		}

		cmdPath := append(append([]string(nil), path...), cmd.Name)
		r.ByPath[strings.Join(cmdPath, " ")] = cmd
		for _, alias := range cmd.Aliases {
			if alias == "" {
				continue
			}
			aliasPath := append(append([]string(nil), path...), alias)
			if _, exists := r.Aliases[strings.Join(aliasPath, " ")]; !exists {
				r.Aliases[strings.Join(aliasPath, " ")] = cmd
			}
		}

		r.indexCommands(cmd, cmdPath, cmd.SubCommands)
	}
}

func (r *Registry) registerBuiltins(commands Commands) {
	r.Builtins["help"] = func(app *App, args []string) error {
		return app.runHelpCommand(args)
	}
	r.visibleBuiltins = append(r.visibleBuiltins, "help")

	if commands.Search("completion") == nil {
		r.Builtins["completion"] = func(app *App, args []string) error {
			return app.runCompletion(args)
		}
		r.visibleBuiltins = append(r.visibleBuiltins, "completion")
	}
	if commands.Search("spec") == nil {
		r.Builtins["spec"] = func(app *App, args []string) error {
			return app.runSpec(args)
		}
		r.visibleBuiltins = append(r.visibleBuiltins, "spec")
	}
	if commands.Search("docs") == nil {
		r.Builtins["docs"] = func(app *App, args []string) error {
			return app.runDocs(args)
		}
		r.visibleBuiltins = append(r.visibleBuiltins, "docs")
	}
	if commands.Search("__complete") == nil {
		r.Builtins["__complete"] = func(app *App, args []string) error {
			return app.runComplete(args)
		}
	}
}

func (r *Registry) Builtin(name string) (BuiltinHandler, bool) {
	if r == nil {
		return nil, false
	}
	handler, ok := r.Builtins[name]
	return handler, ok
}

func (r *Registry) VisibleBuiltins() []string {
	if r == nil || len(r.visibleBuiltins) == 0 {
		return nil
	}
	return append([]string(nil), r.visibleBuiltins...)
}

func (r *Registry) Lookup(parent *Command, name string) *Command {
	if r == nil {
		return nil
	}
	index := r.commandsByParent[parent]
	if len(index) == 0 {
		return nil
	}
	return index[name]
}

func (r *Registry) ResolveCommand(args []string) ([]*Command, *Command, []string, error) {
	if r == nil {
		return nil, nil, nil, fmt.Errorf("%w, no command registry", ErrNotFound)
	}
	if len(args) == 0 {
		return nil, nil, nil, fmt.Errorf("%w, no command provided", ErrNotFound)
	}

	parent := (*Command)(nil)
	path := make([]*Command, 0, len(args))
	cmd := r.Lookup(parent, args[0])
	if cmd == nil {
		return nil, nil, nil, fmt.Errorf("%w, unknown command %q", ErrNotFound, args[0])
	}

	cmd.alias = args[0]
	path = append(path, cmd)
	for i := 1; i < len(args) && len(cmd.SubCommands) > 0; i++ {
		next := r.Lookup(cmd, args[i])
		if next == nil {
			if !cmd.Runnable() {
				return nil, nil, nil, fmt.Errorf("%w, unknown command %q", ErrNotFound, args[i])
			}
			return path, cmd, args[i:], nil
		}
		next.alias = args[i]
		cmd = next
		path = append(path, cmd)
	}

	return path, cmd, args[len(path):], nil
}

func findCommand(app *App, parent *Command, cmds Commands, args []string) (*Command, []string, error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("%w, no command provided", ErrNotFound)
	}

	switch {
	case parent != nil:
		registry := newRegistry(&Command{SubCommands: parent.SubCommands})
		path, cmd, remaining, err := registry.ResolveCommand(args)
		if err != nil {
			return nil, nil, err
		}
		if len(path) > 0 {
			cmd.alias = path[len(path)-1].alias
		}
		return cmd, remaining, nil
	case app != nil:
		path, cmd, remaining, err := app.registry().ResolveCommand(args)
		if err != nil {
			return nil, nil, err
		}
		if len(path) > 0 {
			cmd.alias = path[len(path)-1].alias
		}
		return cmd, remaining, nil
	default:
		registry := newRegistry(&Command{SubCommands: cmds})
		path, cmd, remaining, err := registry.ResolveCommand(args)
		if err != nil {
			return nil, nil, err
		}
		if len(path) > 0 {
			cmd.alias = path[len(path)-1].alias
		}
		return cmd, remaining, nil
	}
}

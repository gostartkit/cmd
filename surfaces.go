package cmd

type Surface string

const (
	SurfaceCLI  Surface = "cli"
	SurfaceREPL Surface = "repl"
)

type CommandSurface struct {
	UsageLine          string
	Short              string
	Long               string
	Group              string
	Deprecated         string
	Aliases            []string
	Examples           []string
	Hidden             *bool
	ReplacePositionals bool
	Positionals        []PositionalArg
	Extensions         map[string]any
}

type PositionalSurface struct {
	Name          string
	Usage         string
	Required      *bool
	Variadic      *bool
	Enum          []string
	Example       string
	Kind          string
	CompletionKey string
	Extensions    map[string]any
}

type FlagSurface struct {
	Usage         string
	Default       string
	Category      string
	Enum          []string
	Required      *bool
	Hidden        *bool
	Deprecated    string
	Example       string
	Kind          string
	Repeatable    *bool
	CompletionKey string
	Extensions    map[string]any
}

func cloneCommandSurfaces(surfaces map[Surface]CommandSurface) map[Surface]CommandSurface {
	if len(surfaces) == 0 {
		return nil
	}
	cloned := make(map[Surface]CommandSurface, len(surfaces))
	for surface, override := range surfaces {
		cloned[surface] = cloneCommandSurface(override)
	}
	return cloned
}

func cloneCommandSurface(surface CommandSurface) CommandSurface {
	cloned := surface
	if len(surface.Aliases) > 0 {
		cloned.Aliases = append([]string(nil), surface.Aliases...)
	}
	if len(surface.Examples) > 0 {
		cloned.Examples = append([]string(nil), surface.Examples...)
	}
	if len(surface.Positionals) > 0 {
		cloned.Positionals = clonePositionals(surface.Positionals)
	} else if surface.ReplacePositionals {
		cloned.Positionals = []PositionalArg{}
	}
	cloned.Extensions = cloneExtensions(surface.Extensions)
	return cloned
}

func clonePositionalSurfaces(surfaces map[Surface]PositionalSurface) map[Surface]PositionalSurface {
	if len(surfaces) == 0 {
		return nil
	}
	cloned := make(map[Surface]PositionalSurface, len(surfaces))
	for surface, override := range surfaces {
		cloned[surface] = clonePositionalSurface(override)
	}
	return cloned
}

func clonePositionalSurface(surface PositionalSurface) PositionalSurface {
	cloned := surface
	if len(surface.Enum) > 0 {
		cloned.Enum = append([]string(nil), surface.Enum...)
	}
	cloned.Extensions = cloneExtensions(surface.Extensions)
	return cloned
}

func cloneFlagSurfaces(surfaces map[Surface]FlagSurface) map[Surface]FlagSurface {
	if len(surfaces) == 0 {
		return nil
	}
	cloned := make(map[Surface]FlagSurface, len(surfaces))
	for surface, override := range surfaces {
		cloned[surface] = cloneFlagSurface(override)
	}
	return cloned
}

func cloneFlagSurface(surface FlagSurface) FlagSurface {
	cloned := surface
	if len(surface.Enum) > 0 {
		cloned.Enum = append([]string(nil), surface.Enum...)
	}
	cloned.Extensions = cloneExtensions(surface.Extensions)
	return cloned
}

func clonePositionals(positionals []PositionalArg) []PositionalArg {
	if len(positionals) == 0 {
		return nil
	}
	cloned := make([]PositionalArg, 0, len(positionals))
	for _, positional := range positionals {
		copyPositional := positional
		copyPositional.Enum = append([]string(nil), positional.Enum...)
		copyPositional.Extensions = cloneExtensions(positional.Extensions)
		copyPositional.Surfaces = clonePositionalSurfaces(positional.Surfaces)
		cloned = append(cloned, copyPositional)
	}
	return cloned
}

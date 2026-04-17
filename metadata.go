package cmd

func (a *App) SetExtension(key string, value any) {
	if a == nil {
		return
	}
	if a.Extensions == nil {
		a.Extensions = make(map[string]any)
	}
	a.Extensions[key] = value
}

func (c *Command) SetExtension(key string, value any) {
	if c == nil {
		return
	}
	if c.Extensions == nil {
		c.Extensions = make(map[string]any)
	}
	c.Extensions[key] = value
}

func (p *PositionalArg) SetExtension(key string, value any) {
	if p == nil {
		return
	}
	if p.Extensions == nil {
		p.Extensions = make(map[string]any)
	}
	p.Extensions[key] = value
}

func (f *FlagSet) SetExtension(name, key string, value any) error {
	flag, ok := f.Lookup(name)
	if !ok {
		return f.failf("flag provided but not defined: --%s", name)
	}
	if flag.Extensions == nil {
		flag.Extensions = make(map[string]any)
	}
	flag.Extensions[key] = value
	return nil
}

func cloneExtensions(extensions map[string]any) map[string]any {
	if len(extensions) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(extensions))
	for key, value := range extensions {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneExtensions(typed)
	case []string:
		return append([]string(nil), typed...)
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneValue(item))
		}
		return cloned
	default:
		return value
	}
}

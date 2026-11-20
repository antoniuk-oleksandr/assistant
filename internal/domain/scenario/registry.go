package scenario

// Registry holds the implementations compiled into this executable, independently
// of which implementations have been registered for use in SQLite.
type Registry struct {
	implementations map[string]Implementation
}

func NewRegistry(implementations ...Implementation) Registry {
	byName := make(map[string]Implementation, len(implementations))
	for _, implementation := range implementations {
		byName[implementation.Name] = implementation
	}
	return Registry{implementations: byName}
}

func (r Registry) Resolve(name string) (Implementation, error) {
	implementation, ok := r.implementations[name]
	if !ok {
		return Implementation{}, NotFound(name)
	}
	return implementation, nil
}

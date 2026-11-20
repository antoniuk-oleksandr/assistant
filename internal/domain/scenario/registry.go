package scenario

// Registry holds the implementations compiled into this executable, independently
// of which implementations have been registered for use in SQLite.
type Registry struct {
	implementations map[string]Implementation
	processors      map[string]CollectionProcessor
}

func NewRegistry(implementations ...Implementation) Registry {
	byName := make(map[string]Implementation, len(implementations))
	for _, implementation := range implementations {
		byName[implementation.Name] = implementation
	}
	return Registry{implementations: byName}
}

// NewRegistryWithProcessors registers compiled implementations and their
// scenario-owned collection rules. The implementation metadata remains
// separate from the processor so callers that only need registration do not
// depend on processing details.
func NewRegistryWithProcessors(implementations []Implementation, processors map[string]CollectionProcessor) Registry {
	byName := make(map[string]Implementation, len(implementations))
	for _, implementation := range implementations {
		byName[implementation.Name] = implementation
	}
	byProcessor := make(map[string]CollectionProcessor, len(processors))
	for name, processor := range processors {
		byProcessor[name] = processor
	}
	return Registry{implementations: byName, processors: byProcessor}
}

func (r Registry) Resolve(name string) (Implementation, error) {
	implementation, ok := r.implementations[name]
	if !ok {
		return Implementation{}, NotFound(name)
	}
	return implementation, nil
}

func (r Registry) ResolveProcessor(name string) (CollectionProcessor, error) {
	if _, err := r.Resolve(name); err != nil {
		return nil, err
	}
	processor, ok := r.processors[name]
	if !ok || processor == nil {
		return nil, NotFound(name)
	}
	return processor, nil
}

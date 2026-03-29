package sources

import (
	"fmt"
	"sync"

	"github.com/ystkfujii/tring/internal/domain/model"
)

var (
	factoryMu sync.RWMutex
	factories = make(map[string]model.SourceFactory)
)

// Register registers a source factory for a given type.
// This should be called during init() in each source implementation.
func Register(kind string, factory model.SourceFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()

	if factory == nil {
		panic("sources: Register factory is nil")
	}
	if _, dup := factories[kind]; dup {
		panic("sources: Register called twice for factory " + kind)
	}
	factories[kind] = factory
}

// Get returns the factory for the given source type.
func Get(kind string) (model.SourceFactory, error) {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	factory, ok := factories[kind]
	if !ok {
		return nil, fmt.Errorf("unknown source type: %q", kind)
	}
	return factory, nil
}

// RegisteredTypes returns a list of all registered source types.
func RegisteredTypes() []string {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	types := make([]string, 0, len(factories))
	for k := range factories {
		types = append(types, k)
	}
	return types
}

// IsRegistered returns true if the given source type is registered.
func IsRegistered(kind string) bool {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	_, ok := factories[kind]
	return ok
}

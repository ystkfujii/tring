package resolver

import (
	"fmt"
	"sync"

	"github.com/ystkfujii/tring/internal/domain/model"
)

var (
	factoryMu sync.RWMutex
	factories = make(map[string]model.ResolverFactory)
)

// Register registers a resolver factory for a given type.
// This should be called during init() in each resolver implementation.
func Register(kind string, factory model.ResolverFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()

	if factory == nil {
		panic("resolver: Register factory is nil")
	}
	if _, dup := factories[kind]; dup {
		panic("resolver: Register called twice for factory " + kind)
	}
	factories[kind] = factory
}

// Get returns the factory for the given resolver type.
func Get(kind string) (model.ResolverFactory, error) {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	factory, ok := factories[kind]
	if !ok {
		return nil, fmt.Errorf("unknown resolver type: %q", kind)
	}
	return factory, nil
}

// RegisteredTypes returns a list of all registered resolver types.
func RegisteredTypes() []string {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	types := make([]string, 0, len(factories))
	for k := range factories {
		types = append(types, k)
	}
	return types
}

// IsRegistered returns true if the given resolver type is registered.
func IsRegistered(kind string) bool {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	_, ok := factories[kind]
	return ok
}

// RegisterForTest registers a resolver factory for testing purposes.
// Unlike Register, it allows overwriting existing factories.
func RegisterForTest(kind string, factory model.ResolverFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()

	factories[kind] = factory
}

// UnregisterForTest removes a resolver factory (for test cleanup).
func UnregisterForTest(kind string) {
	factoryMu.Lock()
	defer factoryMu.Unlock()

	delete(factories, kind)
}

package config

import (
	"sync"
)

var (
	resolverConfigMu         sync.RWMutex
	resolverConfigValidators = make(map[string]ConfigValidator)
)

// RegisterResolverConfigValidator registers a validator for a resolver type.
func RegisterResolverConfigValidator(kind string, validator ConfigValidator) {
	resolverConfigMu.Lock()
	defer resolverConfigMu.Unlock()

	if validator == nil {
		panic("config: RegisterResolverConfigValidator validator is nil")
	}
	if _, dup := resolverConfigValidators[kind]; dup {
		panic("config: RegisterResolverConfigValidator called twice for " + kind)
	}
	resolverConfigValidators[kind] = validator
}

// ResetResolverConfigValidatorsForTest clears the resolver config validator registry for testing.
func ResetResolverConfigValidatorsForTest() {
	resolverConfigMu.Lock()
	defer resolverConfigMu.Unlock()
	resolverConfigValidators = make(map[string]ConfigValidator)
}

// ValidateResolverConfig validates a resolver's configuration
// by delegating to the registered validator for that resolver type.
func ValidateResolverConfig(kind string, raw map[string]interface{}) error {
	resolverConfigMu.RLock()
	validator, ok := resolverConfigValidators[kind]
	resolverConfigMu.RUnlock()

	if !ok {
		// If no validator is registered, skip validation.
		// This is acceptable for resolver types that don't require config validation.
		return nil
	}

	return validator(raw)
}

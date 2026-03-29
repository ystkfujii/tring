package config

import (
	"sync"
)

// ConfigValidator validates a raw config map.
// Each source implementation registers a validator function.
type ConfigValidator func(raw map[string]interface{}) error

var (
	sourceConfigMu         sync.RWMutex
	sourceConfigValidators = make(map[string]ConfigValidator)
)

// RegisterSourceConfigValidator registers a validator for a source type.
// This should be called during init() in each source implementation.
func RegisterSourceConfigValidator(kind string, validator ConfigValidator) {
	sourceConfigMu.Lock()
	defer sourceConfigMu.Unlock()

	if validator == nil {
		panic("config: RegisterSourceConfigValidator validator is nil")
	}
	if _, dup := sourceConfigValidators[kind]; dup {
		panic("config: RegisterSourceConfigValidator called twice for " + kind)
	}
	sourceConfigValidators[kind] = validator
}

// ValidateSourceConfig validates a source's configuration
// by delegating to the registered validator for that source type.
func ValidateSourceConfig(kind string, raw map[string]interface{}) error {
	sourceConfigMu.RLock()
	validator, ok := sourceConfigValidators[kind]
	sourceConfigMu.RUnlock()

	if !ok {
		// If no validator is registered, skip validation.
		// This is acceptable for source types that don't require config validation.
		return nil
	}

	return validator(raw)
}

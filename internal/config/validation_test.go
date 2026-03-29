package config_test

import (
	"strings"
	"testing"

	"github.com/ystkfujii/tring/internal/config"

	// Import bootstrap to register source config decoders
	_ "github.com/ystkfujii/tring/pkg/impl/bootstrap"
)

func TestValidation(t *testing.T) {
	validator := config.NewValidator(
		[]string{"gomod", "envfile"},
		[]string{"goproxy"},
	)

	t.Run("valid config", func(t *testing.T) {
		cfg := &config.Config{
			Version: 1,
			Groups: []config.Group{
				{
					Name:     "test",
					Resolver: "goproxy",
					Sources: []config.RawSource{
						{
							Type: "gomod",
							Config: map[string]interface{}{
								"manifest_paths": []interface{}{"go.mod"},
							},
						},
					},
				},
			},
		}

		errs := validator.Validate(cfg)
		if !errs.IsEmpty() {
			t.Errorf("Validate() = %v, want no errors", errs)
		}
	})

	t.Run("invalid version", func(t *testing.T) {
		cfg := &config.Config{
			Version: 99,
			Groups: []config.Group{
				{
					Name: "test",
					Sources: []config.RawSource{{
						Type: "gomod",
						Config: map[string]interface{}{
							"manifest_paths": []interface{}{"go.mod"},
						},
					}},
				},
			},
		}

		errs := validator.Validate(cfg)
		if errs.IsEmpty() {
			t.Error("Validate() returned no errors for invalid version")
		}
		if !strings.Contains(errs.Error(), "version") {
			t.Errorf("Validate() error should mention 'version': %v", errs)
		}
	})

	t.Run("duplicate group names", func(t *testing.T) {
		cfg := &config.Config{
			Version: 1,
			Groups: []config.Group{
				{
					Name: "test",
					Sources: []config.RawSource{{
						Type: "gomod",
						Config: map[string]interface{}{
							"manifest_paths": []interface{}{"go.mod"},
						},
					}},
				},
				{
					Name: "test",
					Sources: []config.RawSource{{
						Type: "gomod",
						Config: map[string]interface{}{
							"manifest_paths": []interface{}{"go.mod"},
						},
					}},
				},
			},
		}

		errs := validator.Validate(cfg)
		if errs.IsEmpty() {
			t.Error("Validate() returned no errors for duplicate group names")
		}
		if !strings.Contains(errs.Error(), "duplicate") {
			t.Errorf("Validate() error should mention 'duplicate': %v", errs)
		}
	})

	t.Run("unknown source type", func(t *testing.T) {
		cfg := &config.Config{
			Version: 1,
			Groups: []config.Group{
				{
					Name: "test",
					Sources: []config.RawSource{
						{Type: "unknown", Config: map[string]interface{}{}},
					},
				},
			},
		}

		errs := validator.Validate(cfg)
		if errs.IsEmpty() {
			t.Error("Validate() returned no errors for unknown source type")
		}
	})

	t.Run("unknown resolver type", func(t *testing.T) {
		cfg := &config.Config{
			Version: 1,
			Groups: []config.Group{
				{
					Name:     "test",
					Resolver: "unknown",
					Sources: []config.RawSource{
						{
							Type: "gomod",
							Config: map[string]interface{}{
								"manifest_paths": []interface{}{"go.mod"},
							},
						},
					},
				},
			},
		}

		errs := validator.Validate(cfg)
		if errs.IsEmpty() {
			t.Error("Validate() returned no errors for unknown resolver type")
		}
	})

	t.Run("invalid strategy", func(t *testing.T) {
		cfg := &config.Config{
			Version: 1,
			Groups: []config.Group{
				{
					Name: "test",
					Sources: []config.RawSource{
						{
							Type: "gomod",
							Config: map[string]interface{}{
								"manifest_paths": []interface{}{"go.mod"},
							},
						},
					},
					Policy: &config.Policy{
						Selection: &config.Selection{
							Strategy: "invalid",
						},
					},
				},
			},
		}

		errs := validator.Validate(cfg)
		if errs.IsEmpty() {
			t.Error("Validate() returned no errors for invalid strategy")
		}
	})

	t.Run("envfile missing variables", func(t *testing.T) {
		cfg := &config.Config{
			Version: 1,
			Groups: []config.Group{
				{
					Name: "test",
					Sources: []config.RawSource{
						{
							Type: "envfile",
							Config: map[string]interface{}{
								"file_paths": []interface{}{"versions.env"},
								// missing variables
							},
						},
					},
				},
			},
		}

		errs := validator.Validate(cfg)
		if errs.IsEmpty() {
			t.Error("Validate() returned no errors for envfile missing variables")
		}
	})

	t.Run("valid envfile config", func(t *testing.T) {
		cfg := &config.Config{
			Version: 1,
			Groups: []config.Group{
				{
					Name:     "test",
					Resolver: "goproxy",
					Sources: []config.RawSource{
						{
							Type: "envfile",
							Config: map[string]interface{}{
								"file_paths": []interface{}{"versions.env"},
								"variables": []interface{}{
									map[string]interface{}{
										"name":         "FOO",
										"resolve_with": "github.com/foo/bar",
									},
								},
							},
						},
					},
				},
			},
		}

		errs := validator.Validate(cfg)
		if !errs.IsEmpty() {
			t.Errorf("Validate() = %v, want no errors", errs)
		}
	})
}

func TestValidateSelectorPatterns(t *testing.T) {
	validator := config.NewValidator(
		[]string{"gomod"},
		[]string{"goproxy"},
	)

	tests := []struct {
		name     string
		patterns []string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid wildcard",
			patterns: []string{"*"},
			wantErr:  false,
		},
		{
			name:     "valid prefix wildcard",
			patterns: []string{"k8s.io/*"},
			wantErr:  false,
		},
		{
			name:     "valid exact match",
			patterns: []string{"github.com/spf13/cobra"},
			wantErr:  false,
		},
		{
			name:     "valid multiple patterns",
			patterns: []string{"k8s.io/*", "github.com/foo/bar"},
			wantErr:  false,
		},
		{
			name:     "invalid recursive glob",
			patterns: []string{"k8s.io/**"},
			wantErr:  true,
			errMsg:   "recursive glob '**' is not supported",
		},
		{
			name:     "invalid recursive glob with suffix",
			patterns: []string{"**/api"},
			wantErr:  true,
			errMsg:   "recursive glob '**' is not supported",
		},
		{
			name:     "invalid multiple wildcards",
			patterns: []string{"*/*"},
			wantErr:  true,
			errMsg:   "multiple wildcards",
		},
		{
			name:     "invalid wildcard position",
			patterns: []string{"k8s.io/*/foo"},
			wantErr:  true,
			errMsg:   "wildcard only supported as '*' or 'prefix/*'",
		},
		{
			name:     "invalid prefix wildcard",
			patterns: []string{"*foo"},
			wantErr:  true,
			errMsg:   "wildcard only supported as '*' or 'prefix/*'",
		},
		{
			name:     "empty pattern",
			patterns: []string{""},
			wantErr:  true,
			errMsg:   "empty pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Version: 1,
				Groups: []config.Group{
					{
						Name: "test",
						Sources: []config.RawSource{
							{
								Type: "gomod",
								Config: map[string]interface{}{
									"manifest_paths": []interface{}{"go.mod"},
								},
							},
						},
						Selectors: &config.Selectors{
							Include: &config.SelectorPatterns{
								ModulePatterns: tt.patterns,
							},
						},
					},
				},
			}

			errs := validator.Validate(cfg)
			if tt.wantErr {
				if errs.IsEmpty() {
					t.Errorf("Validate() returned no errors for pattern %v", tt.patterns)
				} else if tt.errMsg != "" && !strings.Contains(errs.Error(), tt.errMsg) {
					t.Errorf("Validate() error %q should contain %q", errs.Error(), tt.errMsg)
				}
			} else {
				if !errs.IsEmpty() {
					t.Errorf("Validate() = %v, want no errors", errs)
				}
			}
		})
	}
}

func TestValidateGroupExists(t *testing.T) {
	cfg := &config.Config{
		Version: 1,
		Groups: []config.Group{
			{Name: "exists"},
		},
	}

	if err := config.ValidateGroupExists(cfg, "exists"); err != nil {
		t.Errorf("ValidateGroupExists() = %v, want nil", err)
	}

	if err := config.ValidateGroupExists(cfg, "not-exists"); err == nil {
		t.Error("ValidateGroupExists() = nil, want error")
	}
}

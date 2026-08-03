package app

import (
	"testing"
	"time"

	"ccLoad/internal/model"
)

func newRuntimeConfigTestService(values map[string]string) *ConfigService {
	settings := make(map[string]*model.SystemSetting, len(values))
	for key, value := range values {
		settings[key] = &model.SystemSetting{Key: key, Value: value}
	}
	return &ConfigService{cache: settings, loaded: true}
}

func TestLoadServerRuntimeConfigUpstreamConnectionMaxAge(t *testing.T) {
	t.Parallel()

	cfg := loadServerRuntimeConfig(newRuntimeConfigTestService(map[string]string{
		"upstream_connection_reuse_limit_seconds": "540",
	}))
	if cfg.UpstreamConnectionMaxAge != 9*time.Minute {
		t.Fatalf("UpstreamConnectionMaxAge=%v, want 9m", cfg.UpstreamConnectionMaxAge)
	}

	for name, value := range map[string]string{"disabled": "0", "invalid": "-1"} {
		t.Run(name, func(t *testing.T) {
			got := loadServerRuntimeConfig(newRuntimeConfigTestService(map[string]string{
				"upstream_connection_reuse_limit_seconds": value,
			})).UpstreamConnectionMaxAge
			if got != 0 {
				t.Fatalf("UpstreamConnectionMaxAge=%v, want 0", got)
			}
		})
	}
}

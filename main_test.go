package main

import (
	"testing"
	"time"
)

func TestGetServerTLSConfig(t *testing.T) {
	tests := []struct {
		name      string
		enabled   string
		certFile  string
		keyFile   string
		want      serverTLSConfig
		wantError bool
	}{
		{
			name: "disabled by default",
			want: serverTLSConfig{},
		},
		{
			name:     "enabled with certificate files",
			enabled:  "true",
			certFile: " /etc/ccload/cert.pem ",
			keyFile:  " /etc/ccload/key.pem ",
			want: serverTLSConfig{
				enabled:  true,
				certFile: "/etc/ccload/cert.pem",
				keyFile:  "/etc/ccload/key.pem",
			},
		},
		{
			name:      "enabled without certificate",
			enabled:   "1",
			keyFile:   "/etc/ccload/key.pem",
			want:      serverTLSConfig{enabled: true, keyFile: "/etc/ccload/key.pem"},
			wantError: true,
		},
		{
			name:      "enabled without key",
			enabled:   "yes",
			certFile:  "/etc/ccload/cert.pem",
			want:      serverTLSConfig{enabled: true, certFile: "/etc/ccload/cert.pem"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CCLOAD_TLS_ENABLED", tt.enabled)
			t.Setenv("CCLOAD_TLS_CERT_FILE", tt.certFile)
			t.Setenv("CCLOAD_TLS_KEY_FILE", tt.keyFile)

			got := getServerTLSConfig()
			if got != tt.want {
				t.Fatalf("getServerTLSConfig() = %#v, want %#v", got, tt.want)
			}
			if (got.validate() != nil) != tt.wantError {
				t.Fatalf("validate() error = %v, want error=%v", got.validate(), tt.wantError)
			}
		})
	}
}

func TestLoadHTTPReadTimeout(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      time.Duration
		wantError bool
	}{
		{name: "default", want: 120 * time.Second},
		{name: "custom", value: "300", want: 300 * time.Second},
		{name: "trimmed", value: " 45 ", want: 45 * time.Second},
		{name: "zero", value: "0", wantError: true},
		{name: "negative", value: "-1", wantError: true},
		{name: "invalid", value: "five", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadHTTPReadTimeout(func(string) string { return tt.value })
			if (err != nil) != tt.wantError {
				t.Fatalf("loadHTTPReadTimeout() error = %v, want error=%v", err, tt.wantError)
			}
			if got != tt.want {
				t.Fatalf("loadHTTPReadTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

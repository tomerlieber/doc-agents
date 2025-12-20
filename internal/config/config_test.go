package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range originalEnv {
			// Parse and restore each env var
			for i, c := range env {
				if c == '=' {
					os.Setenv(env[:i], env[i+1:])
					break
				}
			}
		}
	}()

	// Clear env to test defaults
	os.Clearenv()

	cfg := Load()

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"Port", cfg.Port, 8080},
		{"LogLevel", cfg.LogLevel, "info"},
		{"LLMProvider", cfg.LLMProvider, "openai"},
		{"StoreProvider", cfg.StoreProvider, "postgres"},
		{"QueueProvider", cfg.QueueProvider, "nats"},
		{"LLMModel", cfg.LLMModel, "gpt-4o-mini"},
		{"EmbeddingModel", cfg.EmbeddingModel, "text-embedding-3-large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("expected %s=%v, got %v", tt.name, tt.expected, tt.got)
			}
		})
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Save and restore env
	originalPort := os.Getenv("PORT")
	originalLogLevel := os.Getenv("LOG_LEVEL")
	defer func() {
		os.Setenv("PORT", originalPort)
		os.Setenv("LOG_LEVEL", originalLogLevel)
	}()

	// Set test values
	os.Setenv("PORT", "9090")
	os.Setenv("LOG_LEVEL", "debug")

	cfg := Load()

	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected log level 'debug', got %s", cfg.LogLevel)
	}
}

func TestLoadProviderOverrides(t *testing.T) {
	// Save and restore env
	originalLLM := os.Getenv("LLM_PROVIDER")
	defer func() {
		os.Setenv("LLM_PROVIDER", originalLLM)
	}()

	// Set test values
	os.Setenv("LLM_PROVIDER", "stub")

	cfg := Load()

	if cfg.LLMProvider != "stub" {
		t.Errorf("expected LLM provider 'stub', got %s", cfg.LLMProvider)
	}
}

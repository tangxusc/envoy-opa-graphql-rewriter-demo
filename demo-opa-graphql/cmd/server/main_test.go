package main

import (
	"os"
	"testing"
)

func TestEnvOrDefault_WithEnv(t *testing.T) {
	t.Setenv("TEST_ENV_VAR_XYZ", "custom-value")
	got := envOrDefault("TEST_ENV_VAR_XYZ", "default-value")
	if got != "custom-value" {
		t.Errorf("envOrDefault = %q, want %q", got, "custom-value")
	}
}

func TestEnvOrDefault_WithoutEnv(t *testing.T) {
	os.Unsetenv("TEST_ENV_VAR_UNSET")
	got := envOrDefault("TEST_ENV_VAR_UNSET", "default-value")
	if got != "default-value" {
		t.Errorf("envOrDefault = %q, want %q", got, "default-value")
	}
}

func TestPrintDemoTokens(t *testing.T) {
	// Just ensure it doesn't panic
	printDemoTokens([]byte("test-secret"))
}

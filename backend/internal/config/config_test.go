package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_NoEnvFile_UsesSystemVars(t *testing.T) {
	// Ensure no .env exists in test path
	os.Setenv("TEST_VAR", "hello")
	defer os.Unsetenv("TEST_VAR")

	Load() // Should not panic

	if val := Get("TEST_VAR", ""); val != "hello" {
		t.Errorf("expected 'hello', got '%s'", val)
	}
}

func TestGet_ReturnsDefaultWhenMissing(t *testing.T) {
	if val := Get("NONEXISTENT_VAR_12345", "default_val"); val != "default_val" {
		t.Errorf("expected 'default_val', got '%s'", val)
	}
}

func TestGet_ReturnsEnvVarWhenSet(t *testing.T) {
	os.Setenv("MY_TEST_KEY", "my_value")
	defer os.Unsetenv("MY_TEST_KEY")

	if val := Get("MY_TEST_KEY", "fallback"); val != "my_value" {
		t.Errorf("expected 'my_value', got '%s'", val)
	}
}

func TestLoad_MultiplePaths(t *testing.T) {
	// Create a temporary .env and test that Load() finds it
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	os.WriteFile(envPath, []byte("CUSTOM_PATH_VAR=custom_value\n"), 0644)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	os.Unsetenv("CUSTOM_PATH_VAR")
	Load()

	if val := Get("CUSTOM_PATH_VAR", ""); val != "custom_value" {
		t.Errorf("expected 'custom_value', got '%s'", val)
	}
}
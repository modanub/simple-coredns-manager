package coredns

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CorefileManager struct {
	path string
}

func NewCorefileManager(path string) *CorefileManager {
	return &CorefileManager{path: path}
}

func (m *CorefileManager) Read() (string, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return "", fmt.Errorf("failed to read Corefile: %w", err)
	}
	return string(data), nil
}

func (m *CorefileManager) Write(content string) error {
	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")

	// Ensure trailing newline
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	// Atomic write: write to temp file then rename
	dir := filepath.Dir(m.path)
	tmp, err := os.CreateTemp(dir, ".corefile-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Preserve original permissions
	info, err := os.Stat(m.path)
	if err == nil {
		os.Chmod(tmpPath, info.Mode())
	}

	if err := os.Rename(tmpPath, m.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

func (m *CorefileManager) Validate(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("Corefile cannot be empty")
	}

	// Basic structural validation: check for balanced braces
	open := strings.Count(content, "{")
	close := strings.Count(content, "}")
	if open != close {
		return fmt.Errorf("unbalanced braces: %d opening, %d closing", open, close)
	}

	return nil
}

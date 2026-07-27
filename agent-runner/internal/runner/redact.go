package runner

import (
	"os"
	"path/filepath"
	"strings"
)

type Redactor struct{ values []string }

func NewRedactor(secretRoot string, references []string) Redactor {
	values := make([]string, 0)
	for _, reference := range references {
		entries, err := os.ReadDir(filepath.Join(secretRoot, reference))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			value, err := os.ReadFile(filepath.Join(secretRoot, reference, entry.Name()))
			if err == nil && len(value) >= 4 {
				values = append(values, string(value))
			}
		}
	}
	return Redactor{values: values}
}

func (redactor Redactor) Redact(value string) string {
	for _, secret := range redactor.values {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	return value
}

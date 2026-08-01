package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Redactor struct{ values []string }

func NewRedactor(secretRoot string, references []string) (Redactor, error) {
	values := make([]string, 0)
	for _, reference := range references {
		entries, err := os.ReadDir(filepath.Join(secretRoot, reference))
		if err != nil {
			return Redactor{}, fmt.Errorf("read secret projection %q: %w", reference, err)
		}
		for _, entry := range entries {
			path := filepath.Join(secretRoot, reference, entry.Name())
			if entry.IsDir() {
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 {
				target, err := os.Stat(path)
				if err != nil {
					return Redactor{}, fmt.Errorf("read secret projection %q/%q: %w", reference, entry.Name(), err)
				}
				if target.IsDir() {
					continue
				}
			}
			value, err := os.ReadFile(path)

			if err != nil {
				return Redactor{}, fmt.Errorf("read secret projection %q/%q: %w", reference, entry.Name(), err)
			}
			if len(value) > 0 {
				values = append(values, string(value))
			}
		}
	}
	slices.SortFunc(values, func(left, right string) int { return len(right) - len(left) })
	return Redactor{values: values}, nil
}

func (redactor Redactor) Redact(value string) string {
	for _, secret := range redactor.values {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	return value
}

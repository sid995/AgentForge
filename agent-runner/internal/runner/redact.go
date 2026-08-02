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
	root, err := filepath.Abs(secretRoot)
	if err != nil {
		return Redactor{}, fmt.Errorf("resolve secret root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Redactor{}, fmt.Errorf("resolve secret root: %w", err)
	}
	values := make([]string, 0)
	for _, reference := range references {
		projection, err := secretProjectionPath(root, reference)
		if err != nil {
			return Redactor{}, err
		}
		entries, err := os.ReadDir(projection)
		if err != nil {
			return Redactor{}, fmt.Errorf("read secret projection %q: %w", reference, err)
		}
		for _, entry := range entries {
			path := filepath.Join(projection, entry.Name())
			if entry.IsDir() {
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 {
				path, err = filepath.EvalSymlinks(path)
				if err != nil {
					return Redactor{}, fmt.Errorf("read secret projection %q/%q: %w", reference, entry.Name(), err)
				}
				if !isChildPath(projection, path) {
					return Redactor{}, fmt.Errorf("secret projection %q/%q escapes its mount", reference, entry.Name())
				}
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

func secretProjectionPath(secretRoot, reference string) (string, error) {
	if reference == "" || reference == "." || filepath.IsAbs(reference) || hasParentSegment(reference) {
		return "", fmt.Errorf("secret projection %q is invalid", reference)
	}
	projection := filepath.Join(secretRoot, reference)
	if !isChildPath(secretRoot, projection) {
		return "", fmt.Errorf("secret projection %q escapes secret root", reference)
	}
	resolved, err := filepath.EvalSymlinks(projection)
	if err != nil {
		return "", fmt.Errorf("read secret projection %q: %w", reference, err)
	}
	if !isChildPath(secretRoot, resolved) {
		return "", fmt.Errorf("secret projection %q escapes secret root", reference)
	}
	info, err := os.Lstat(projection)
	if err != nil {
		return "", fmt.Errorf("read secret projection %q: %w", reference, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("secret projection %q must be a non-symlink directory", reference)
	}
	return resolved, nil
}

func hasParentSegment(path string) bool {
	return slices.ContainsFunc(strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }), func(segment string) bool { return segment == ".." })
}

func isChildPath(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (redactor Redactor) Redact(value string) string {
	for _, secret := range redactor.values {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	return value
}

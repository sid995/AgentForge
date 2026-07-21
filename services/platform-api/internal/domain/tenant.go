package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// Tenant is the top-level ownership boundary for tenant-owned data.
type Tenant struct {
	ID        uuid.UUID
	Slug      string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewTenant validates tenant attributes and assigns a time-sortable UUIDv7.
func NewTenant(slug, name string, now time.Time) (Tenant, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	name = strings.TrimSpace(name)
	if !slugPattern.MatchString(slug) {
		return Tenant{}, fmt.Errorf("tenant slug must be 3 to 63 lowercase letters, numbers, or hyphens")
	}
	if name == "" || len(name) > 120 {
		return Tenant{}, fmt.Errorf("tenant name must contain 1 to 120 characters")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Tenant{}, fmt.Errorf("generate tenant ID: %w", err)
	}
	now = now.UTC()
	return Tenant{ID: id, Slug: slug, Name: name, CreatedAt: now, UpdatedAt: now}, nil
}

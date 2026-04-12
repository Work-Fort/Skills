package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// NewID returns a prefixed UUID string: "<prefix>_<uuid>".
func NewID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, uuid.New().String())
}

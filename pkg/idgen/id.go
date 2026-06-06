package idgen

import "github.com/google/uuid"

func New(prefix string) string {
	id := uuid.NewString()
	if prefix == "" {
		return id
	}
	return prefix + "_" + id
}

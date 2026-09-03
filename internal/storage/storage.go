package storage

import (
	"context"
	"fmt"

	"github.com/mopga/mythblog/internal/config"
)

type ObjectStorage interface {
	Put(ctx context.Context, key string, data []byte, contentType string) (string, error)
	Delete(ctx context.Context, key string) error
	URL(key string) string
}

func FromConfig(cfg config.Config) (ObjectStorage, error) {
	switch cfg.StorageDriver {
	case "local", "":
		return NewLocal(cfg.LocalStoragePath, cfg.PublicBaseURL+"/media"), nil
	case "r2":
		return NewR2(cfg)
	default:
		return nil, fmt.Errorf("unknown storage driver %q", cfg.StorageDriver)
	}
}

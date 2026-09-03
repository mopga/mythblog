package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Local struct{ root, baseURL string }

func NewLocal(root, baseURL string) *Local {
	return &Local{root: root, baseURL: strings.TrimRight(baseURL, "/")}
}
func safePath(root, key string) (string, error) {
	if key == "" || strings.Contains(key, "\\") {
		return "", fmt.Errorf("invalid object key")
	}
	clean := filepath.Clean(key)
	if clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid object key")
	}
	path := filepath.Join(root, clean)
	return path, nil
}
func (l *Local) Put(_ context.Context, key string, data []byte, _ string) (string, error) {
	path, err := safePath(l.root, key)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	temp := path + ".new"
	if err = os.WriteFile(temp, data, 0o600); err != nil {
		return "", err
	}
	if err = os.Rename(temp, path); err != nil {
		return "", err
	}
	return l.URL(key), nil
}
func (l *Local) Delete(_ context.Context, key string) error {
	path, err := safePath(l.root, key)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
func (l *Local) URL(key string) string { return l.baseURL + "/" + key }
func (l *Local) Root() string          { return l.root }

// Package atomicfile stores private state without exposing partial writes.
package atomicfile

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func WriteJSON(name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return Write(name, append(data, '\n'))
}

func Write(name string, data []byte) error {
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".domru-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), name); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

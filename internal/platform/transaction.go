// Package platform owns reversible, journaled host networking changes.
package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotApplied indicates a rejected operation that provably made no changes.
var ErrNotApplied = errors.New("network operation rejected before mutation")

type Command struct {
	Name string   `json:"name"`
	Args []string `json:"args,omitempty"`
}
type Step struct {
	Do   Command `json:"do"`
	Undo Command `json:"undo"`
}
type Runner interface{ Run(string, ...string) error }

// Transaction writes undo operations before applying changes. The caller serializes use.
type Transaction struct {
	Path   string
	Runner Runner
}
type journal struct {
	Version int       `json:"version"`
	Pending []Command `json:"pending"`
}

func AtomicWrite(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".conflux-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func (t *Transaction) save(j journal) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return AtomicWrite(t.Path, b)
}
func (t *Transaction) Apply(steps []Step) error {
	if _, err := os.Stat(t.Path); err == nil {
		return errors.New("network recovery required before activation")
	} else if !os.IsNotExist(err) {
		return err
	}
	j := journal{Version: 1}
	for _, s := range steps {
		j.Pending = append(j.Pending, s.Undo)
		if err := t.save(j); err != nil {
			return errors.Join(err, t.Recover())
		}
		if err := t.Runner.Run(s.Do.Name, s.Do.Args...); err != nil {
			if errors.Is(err, ErrNotApplied) {
				j.Pending = j.Pending[:len(j.Pending)-1]
				if saveErr := t.save(j); saveErr != nil {
					return errors.Join(err, saveErr)
				}
			}
			return errors.Join(fmt.Errorf("network activation: %w", err), t.Recover())
		}
	}
	return nil
}
func (t *Transaction) Recover() error {
	b, err := os.ReadFile(t.Path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(b) > 1<<20 {
		return errors.New("invalid network journal size")
	}
	var j journal
	if err = json.Unmarshal(b, &j); err != nil {
		return err
	}
	if j.Version != 1 {
		return errors.New("unsupported network journal version")
	}
	for len(j.Pending) > 0 {
		i := len(j.Pending) - 1
		c := j.Pending[i]
		if err = t.Runner.Run(c.Name, c.Args...); err != nil {
			return fmt.Errorf("network recovery incomplete: %w", err)
		}
		j.Pending = j.Pending[:i]
		if err = t.save(j); err != nil {
			return err
		}
	}
	return os.Remove(t.Path)
}

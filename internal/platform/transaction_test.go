package platform

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type recorder struct {
	calls []string
	fail  string
}

func (r *recorder) Run(name string, args ...string) error {
	r.calls = append(r.calls, name)
	if name == r.fail {
		return errors.New("failure")
	}
	return nil
}
func TestRollbackAfterPartialActivation(t *testing.T) {
	r := &recorder{fail: "second"}
	p := filepath.Join(t.TempDir(), "state.json")
	tx := Transaction{Path: p, Runner: r}
	err := tx.Apply([]Step{{Do: Command{Name: "first"}, Undo: Command{Name: "undo-first"}}, {Do: Command{Name: "second"}, Undo: Command{Name: "undo-second"}}})
	if err == nil {
		t.Fatal("expected failure")
	}
	// Write-ahead includes the possibly partially executed failing operation.
	if !reflect.DeepEqual(r.calls, []string{"first", "second", "undo-second", "undo-first"}) {
		t.Fatal(r.calls)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("journal not removed: %v", err)
	}
}
func TestRecoveryIsIdempotent(t *testing.T) {
	r := &recorder{}
	p := filepath.Join(t.TempDir(), "state.json")
	tx := Transaction{Path: p, Runner: r}
	if err := tx.Apply([]Step{{Do: Command{Name: "one"}, Undo: Command{Name: "undo"}}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Recover(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Recover(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.calls, []string{"one", "undo"}) {
		t.Fatal(r.calls)
	}
}
func TestRecoveryRetainsFailedUndo(t *testing.T) {
	r := &recorder{}
	p := filepath.Join(t.TempDir(), "state.json")
	tx := Transaction{Path: p, Runner: r}
	if err := tx.Apply([]Step{{Do: Command{Name: "one"}, Undo: Command{Name: "undo"}}}); err != nil {
		t.Fatal(err)
	}
	r.fail = "undo"
	if tx.Recover() == nil {
		t.Fatal("expected recovery failure")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("lost recovery journal", err)
	}
	r.fail = ""
	if err := tx.Recover(); err != nil {
		t.Fatal(err)
	}
}

type noMutationRunner struct{ calls []string }

func (r *noMutationRunner) Run(name string, args ...string) error {
	r.calls = append(r.calls, name)
	if name == "conflict" {
		return ErrNotApplied
	}
	return nil
}
func TestCreationConflictNeverRemovesForeignResource(t *testing.T) {
	r := &noMutationRunner{}
	tx := Transaction{Path: filepath.Join(t.TempDir(), "state.json"), Runner: r}
	if tx.Apply([]Step{{Do: Command{Name: "conflict"}, Undo: Command{Name: "remove-foreign"}}}) == nil {
		t.Fatal("missing conflict")
	}
	if !reflect.DeepEqual(r.calls, []string{"conflict"}) {
		t.Fatal(r.calls)
	}
}

package control

import (
	"encoding/json"
	"errors"
	"github.com/filiprek/conflux-vpn/internal/platform"
	"os"
)

type EncryptedStore struct{ Path string }

func (s EncryptedStore) Load() (Profile, error) {
	var p Profile
	b, e := os.ReadFile(s.Path)
	if os.IsNotExist(e) {
		return p, nil
	}
	if e != nil {
		return p, e
	}
	if len(b) > 1<<20 {
		return p, errors.New("credential file too large")
	}
	b, e = platform.Unprotect(b)
	if e != nil {
		return p, e
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return p, errors.New("credential file corrupt")
	}
	return p, nil
}
func (s EncryptedStore) Save(p Profile) error {
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	b, e = platform.Protect(b)
	if e != nil {
		return e
	}
	return platform.AtomicWrite(s.Path, b)
}

// Package ledger provides the versioned, atomically replaced local JSON store.
package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spokespan/spokespan/internal/survey"
)

const CurrentVersion = 1

// Ledger is the complete on-disk document.
type Ledger struct {
	Version int             `json:"version"`
	Surveys []survey.Survey `json:"surveys"`
}

// New returns an empty current-version ledger.
func New() Ledger {
	return Ledger{Version: CurrentVersion, Surveys: make([]survey.Survey, 0)}
}

// Store identifies one local ledger file.
type Store struct {
	Path string
}

func NewStore(path string) Store {
	return Store{Path: path}
}

// Load reads and validates a ledger. A missing path is the only empty-ledger
// case; all other read and decode failures are returned to the caller.
func (s Store) Load() (Ledger, error) {
	if s.Path == "" {
		return Ledger{}, errors.New("ledger path is required")
	}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return Ledger{}, fmt.Errorf("read ledger %q: %w", s.Path, err)
	}
	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return Ledger{}, fmt.Errorf("decode ledger %q: %w", s.Path, err)
	}
	if err := l.Validate(); err != nil {
		return Ledger{}, fmt.Errorf("validate ledger %q: %w", s.Path, err)
	}
	return l, nil
}

// Save serializes a ledger to a sibling temporary file, syncs it, and then
// atomically renames it over the target.
func (s Store) Save(l Ledger) error {
	if s.Path == "" {
		return errors.New("ledger path is required")
	}
	if err := l.Validate(); err != nil {
		return fmt.Errorf("validate ledger before save: %w", err)
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ledger: %w", err)
	}
	data = append(data, '\n')

	directory := filepath.Dir(s.Path)
	base := filepath.Base(s.Path)
	temporary, err := os.CreateTemp(directory, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary ledger beside %q: %w", s.Path, err)
	}
	temporaryName := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		if temporaryName != "" {
			_ = os.Remove(temporaryName)
		}
	}()

	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary ledger %q: %w", temporaryName, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary ledger %q: %w", temporaryName, err)
	}
	if err := temporary.Close(); err != nil {
		closed = true
		return fmt.Errorf("close temporary ledger %q: %w", temporaryName, err)
	}
	closed = true
	if err := os.Rename(temporaryName, s.Path); err != nil {
		return fmt.Errorf("replace ledger %q: %w", s.Path, err)
	}
	temporaryName = ""
	return nil
}

// Validate checks the document version, survey IDs, and each survey's state.
func (l Ledger) Validate() error {
	if l.Version != CurrentVersion {
		return fmt.Errorf("unsupported ledger version %d", l.Version)
	}
	seen := make(map[string]struct{}, len(l.Surveys))
	for i, s := range l.Surveys {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("survey %d: %w", i, err)
		}
		if _, exists := seen[s.ID]; exists {
			return fmt.Errorf("duplicate survey id %q", s.ID)
		}
		seen[s.ID] = struct{}{}
	}
	return nil
}

// Add appends a new survey if its ID is not already in the ledger.
func (l *Ledger) Add(s survey.Survey) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("add survey: %w", err)
	}
	if _, exists := l.Find(s.ID); exists {
		return fmt.Errorf("survey %q already exists", s.ID)
	}
	l.Surveys = append(l.Surveys, s)
	return nil
}

// Find returns the survey pointer held by the ledger and whether it exists.
func (l *Ledger) Find(id string) (*survey.Survey, bool) {
	for i := range l.Surveys {
		if l.Surveys[i].ID == id {
			return &l.Surveys[i], true
		}
	}
	return nil, false
}

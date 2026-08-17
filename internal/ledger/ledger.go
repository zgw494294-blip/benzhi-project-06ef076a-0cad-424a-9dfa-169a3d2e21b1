// Package ledger provides the versioned, atomically replaced local JSON store.
package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"syscall"

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

// Save merges a ledger with any concurrent updates, serializes it to a sibling
// temporary file, syncs it, and then atomically renames it over the target.
func (s Store) Save(l Ledger) error {
	if s.Path == "" {
		return errors.New("ledger path is required")
	}
	if err := l.Validate(); err != nil {
		return fmt.Errorf("validate ledger before save: %w", err)
	}
	lock, err := os.OpenFile(filepath.Clean(s.Path)+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open ledger lock for %q: %w", s.Path, err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock ledger %q: %w", s.Path, err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	current, err := s.Load()
	if err != nil {
		return err
	}
	l, err = mergeLedgers(current, l)
	if err != nil {
		return err
	}
	return s.save(l)
}

func (s Store) save(l Ledger) error {
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

func mergeLedgers(current, next Ledger) (Ledger, error) {
	indexes := make(map[string]int, len(current.Surveys))
	for i := range current.Surveys {
		indexes[current.Surveys[i].ID] = i
	}
	for _, candidate := range next.Surveys {
		i, exists := indexes[candidate.ID]
		if !exists {
			indexes[candidate.ID] = len(current.Surveys)
			current.Surveys = append(current.Surveys, candidate)
			continue
		}
		merged, err := mergeSurveys(current.Surveys[i], candidate)
		if err != nil {
			return Ledger{}, err
		}
		current.Surveys[i] = merged
	}
	if err := current.Validate(); err != nil {
		return Ledger{}, fmt.Errorf("validate merged ledger: %w", err)
	}
	return current, nil
}

func mergeSurveys(current, next survey.Survey) (survey.Survey, error) {
	if !sameConfiguration(current, next) {
		return survey.Survey{}, fmt.Errorf("merge survey %q: configuration conflict", next.ID)
	}
	readings := make(map[int]float64, len(current.Readings))
	for _, reading := range current.Readings {
		readings[reading.Spoke] = reading.Tension
	}
	for _, reading := range next.Readings {
		if tension, exists := readings[reading.Spoke]; exists {
			if tension != reading.Tension {
				return survey.Survey{}, fmt.Errorf("merge survey %q: spoke %d reading conflict", next.ID, reading.Spoke)
			}
			continue
		}
		current.Readings = append(current.Readings, reading)
		readings[reading.Spoke] = reading.Tension
	}

	switch {
	case current.Status == survey.Tensioning && next.Status != survey.Tensioning:
		current.Status = next.Status
		current.Report = next.Report
	case current.Status != survey.Tensioning && next.Status != survey.Tensioning:
		if current.Status != next.Status || !reflect.DeepEqual(current.Report, next.Report) {
			return survey.Survey{}, fmt.Errorf("merge survey %q: final report conflict", next.ID)
		}
	}
	return current, nil
}

func sameConfiguration(left, right survey.Survey) bool {
	if left.ID != right.ID || left.SpokeCount != right.SpokeCount || left.MinTension != right.MinTension || left.MaxTension != right.MaxTension || left.MaxSpread != right.MaxSpread {
		return false
	}
	if left.WheelLabel == nil || right.WheelLabel == nil {
		return left.WheelLabel == nil && right.WheelLabel == nil
	}
	return *left.WheelLabel == *right.WheelLabel
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

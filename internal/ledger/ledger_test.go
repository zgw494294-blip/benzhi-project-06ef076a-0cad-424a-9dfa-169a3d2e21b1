package ledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spokespan/spokespan/internal/survey"
)

func storedSurvey(t *testing.T, id string, label *string) survey.Survey {
	t.Helper()
	s, err := survey.New(id, 2, 90, 110, 10, label)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStoreMissingAndRoundTripPreserveOptionalLabel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wheel.json")
	store := NewStore(path)
	missing, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if missing.Version != CurrentVersion || len(missing.Surveys) != 0 {
		t.Fatalf("unexpected missing ledger: %#v", missing)
	}
	emptyLabel := ""
	l := New()
	if err := l.Add(storedSurvey(t, "without-label", nil)); err != nil {
		t.Fatal(err)
	}
	if err := l.Add(storedSurvey(t, "empty-label", &emptyLabel)); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(l); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	without, _ := loaded.Find("without-label")
	with, _ := loaded.Find("empty-label")
	if without.WheelLabel != nil || with.WheelLabel == nil || *with.WheelLabel != "" {
		t.Fatalf("optional label distinction was lost: without=%#v with=%#v", without.WheelLabel, with.WheelLabel)
	}
}

func TestLoadDoesNotTreatBadDocumentsAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wheel.json")
	store := NewStore(path)
	for _, tc := range []struct {
		name string
		data string
		want string
	}{
		{name: "malformed", data: "{", want: "decode ledger"},
		{name: "unsupported version", data: `{"version":2,"surveys":[]}`, want: "unsupported ledger version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.Load()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got ledger=%#v err=%v", tc.want, loaded, err)
			}
		})
	}
}

func TestLoadReturnsOtherReadErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger-directory")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(path).Load(); err == nil || !strings.Contains(err.Error(), "read ledger") {
		t.Fatalf("expected directory read error, got %v", err)
	}
}

func TestSaveRemovesTemporaryFileWhenRenameFails(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(path).Save(New()); err == nil {
		t.Fatal("expected rename failure when target is a directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".ledger.json.tmp-") {
			t.Fatalf("temporary file was not removed: %s", entry.Name())
		}
	}
}

func TestAddRejectsDuplicateIDs(t *testing.T) {
	l := New()
	if err := l.Add(storedSurvey(t, "same", nil)); err != nil {
		t.Fatal(err)
	}
	if err := l.Add(storedSurvey(t, "same", nil)); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate ID error, got %v", err)
	}
}

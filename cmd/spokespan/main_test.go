package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spokespan/spokespan/internal/survey"
)

func runCommand(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func TestCLIWorkflowPersistsImmutableReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wheel.json")
	stdout, stderr, code := runCommand(t, "start", "--ledger", path, "--spokes", "3", "--min-tension", "90", "--max-tension", "110", "--max-spread", "5", "--label", "", "--json")
	if code != 0 {
		t.Fatalf("start failed: stdout=%q stderr=%q", stdout, stderr)
	}
	var created survey.Survey
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatal(err)
	}
	if created.WheelLabel == nil || *created.WheelLabel != "" {
		t.Fatalf("supplied empty label was not retained: %#v", created.WheelLabel)
	}
	for _, reading := range []struct {
		spoke   string
		tension string
	}{
		{spoke: "1", tension: "98"},
		{spoke: "2", tension: "100"},
		{spoke: "3", tension: "99"},
	} {
		_, stderr, code = runCommand(t, "measure", "--ledger", path, "--id", created.ID, "--spoke", reading.spoke, "--tension", reading.tension)
		if code != 0 {
			t.Fatalf("measure failed: stderr=%q", stderr)
		}
	}
	_, stderr, code = runCommand(t, "measure", "--ledger", path, "--id", created.ID, "--spoke", "1", "--tension", "100")
	if code == 0 || !strings.Contains(stderr, "already has") {
		t.Fatalf("expected duplicate measurement rejection, code=%d stderr=%q", code, stderr)
	}
	stdout, stderr, code = runCommand(t, "finalize", "--ledger", path, "--id", created.ID, "--json")
	if code != 0 {
		t.Fatalf("finalize failed: stdout=%q stderr=%q", stdout, stderr)
	}
	var finalized survey.Survey
	if err := json.Unmarshal([]byte(stdout), &finalized); err != nil {
		t.Fatal(err)
	}
	if finalized.Status != survey.Balanced || finalized.Report == nil {
		t.Fatalf("unexpected final result: %#v", finalized)
	}
	_, stderr, code = runCommand(t, "finalize", "--ledger", path, "--id", created.ID)
	if code == 0 || !strings.Contains(stderr, "already finalized") {
		t.Fatalf("expected repeated finalization rejection, code=%d stderr=%q", code, stderr)
	}
	stdout, stderr, code = runCommand(t, "show", "--ledger", path, "--id", created.ID, "--json")
	if code != 0 {
		t.Fatalf("show failed: stdout=%q stderr=%q", stdout, stderr)
	}
	var shown survey.Survey
	if err := json.Unmarshal([]byte(stdout), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.Report == nil || shown.Report.Outcome != survey.Balanced || len(shown.Readings) != 3 {
		t.Fatalf("unexpected shown survey: %#v", shown)
	}
}

func TestCLISmokeUsesTemporaryLedger(t *testing.T) {
	stdout, stderr, code := runCommand(t, "smoke")
	if code != 0 || !strings.Contains(stdout, "smoke ok") {
		t.Fatalf("smoke failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestCLIReportsLedgerDecodeErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wheel.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCommand(t, "show", "--ledger", path, "--id", "wheel")
	if code == 0 || !strings.Contains(stderr, "decode ledger") {
		t.Fatalf("expected actionable ledger error, code=%d stderr=%q", code, stderr)
	}
}

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spokespan/spokespan/internal/ledger"
	"github.com/spokespan/spokespan/internal/survey"
)

const defaultLedgerPath = "spokespan-ledger.json"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printUsage(stdout)
		return 0
	}
	path, remaining, err := globalLedger(args)
	if err != nil {
		return writeError(stderr, err)
	}
	if len(remaining) == 0 {
		printUsage(stdout)
		return 0
	}
	command := remaining[0]
	commandArgs := remaining[1:]
	var commandErr error
	switch command {
	case "start":
		commandErr = runStart(path, commandArgs, stdout, stderr)
	case "measure":
		commandErr = runMeasure(path, commandArgs, stdout, stderr)
	case "finalize":
		commandErr = runFinalize(path, commandArgs, stdout, stderr)
	case "show":
		commandErr = runShow(path, commandArgs, stdout, stderr)
	case "smoke":
		commandErr = runSmoke(commandArgs, stdout, stderr)
	default:
		commandErr = fmt.Errorf("unknown command %q", command)
	}
	if commandErr != nil {
		if errors.Is(commandErr, flag.ErrHelp) {
			return 0
		}
		return writeError(stderr, commandErr)
	}
	return 0
}

func runStart(path string, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("start", stderr)
	var spokes int
	var minTension, maxTension, maxSpread float64
	var wheelLabel string
	var labelSet bool
	var jsonOutput bool
	fs.IntVar(&spokes, "spokes", 0, "number of spokes")
	fs.IntVar(&spokes, "spoke-count", 0, "number of spokes")
	fs.Float64Var(&minTension, "min-tension", 0, "minimum acceptable tension")
	fs.Float64Var(&minTension, "min", 0, "minimum acceptable tension")
	fs.Float64Var(&maxTension, "max-tension", 0, "maximum acceptable tension")
	fs.Float64Var(&maxTension, "max", 0, "maximum acceptable tension")
	fs.Float64Var(&maxSpread, "max-spread", 0, "maximum allowed observed spread")
	fs.Float64Var(&maxSpread, "spread", 0, "maximum allowed observed spread")
	fs.StringVar(&wheelLabel, "label", "", "optional wheel label")
	fs.StringVar(&path, "ledger", path, "local ledger path")
	fs.BoolVar(&jsonOutput, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "label" {
			labelSet = true
		}
	})
	if fs.NArg() != 0 {
		return errors.New("start does not accept positional arguments")
	}

	store := ledger.NewStore(path)
	current, err := store.Load()
	if err != nil {
		return err
	}
	id, err := newUniqueID(&current)
	if err != nil {
		return err
	}
	var label *string
	if labelSet {
		label = &wheelLabel
	}
	created, err := survey.New(id, spokes, minTension, maxTension, maxSpread, label)
	if err != nil {
		return err
	}
	if err := current.Add(created); err != nil {
		return err
	}
	if err := store.Save(current); err != nil {
		return err
	}
	if jsonOutput {
		return encodeJSON(stdout, created)
	}
	fmt.Fprintf(stdout, "started %s with %d spokes\n", created.ID, created.SpokeCount)
	return nil
}

func runMeasure(path string, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("measure", stderr)
	var id string
	var spoke int
	var tension float64
	var jsonOutput bool
	fs.StringVar(&id, "id", "", "survey ID")
	fs.IntVar(&spoke, "spoke", 0, "spoke number")
	fs.Float64Var(&tension, "tension", 0, "positive tension reading")
	fs.StringVar(&path, "ledger", path, "local ledger path")
	fs.BoolVar(&jsonOutput, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("measure does not accept positional arguments")
	}
	if id == "" {
		return errors.New("measure requires --id")
	}
	store := ledger.NewStore(path)
	current, err := store.Load()
	if err != nil {
		return err
	}
	target, exists := current.Find(id)
	if !exists {
		return fmt.Errorf("survey %q not found", id)
	}
	if err := target.Measure(spoke, tension); err != nil {
		return err
	}
	if err := store.Save(current); err != nil {
		return err
	}
	if jsonOutput {
		return encodeJSON(stdout, target)
	}
	fmt.Fprintf(stdout, "recorded %s spoke %d (%d/%d)\n", id, spoke, len(target.Readings), target.SpokeCount)
	return nil
}

func runFinalize(path string, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("finalize", stderr)
	var id string
	var jsonOutput bool
	fs.StringVar(&id, "id", "", "survey ID")
	fs.StringVar(&path, "ledger", path, "local ledger path")
	fs.BoolVar(&jsonOutput, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("finalize does not accept positional arguments")
	}
	if id == "" {
		return errors.New("finalize requires --id")
	}
	store := ledger.NewStore(path)
	current, err := store.Load()
	if err != nil {
		return err
	}
	target, exists := current.Find(id)
	if !exists {
		return fmt.Errorf("survey %q not found", id)
	}
	if err := target.Finalize(); err != nil {
		return err
	}
	if err := store.Save(current); err != nil {
		return err
	}
	if jsonOutput {
		return encodeJSON(stdout, target)
	}
	fmt.Fprintf(stdout, "finalized %s: %s\n", id, target.Status)
	return nil
}

func runShow(path string, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("show", stderr)
	var id string
	var jsonOutput bool
	fs.StringVar(&id, "id", "", "survey ID")
	fs.StringVar(&path, "ledger", path, "local ledger path")
	fs.BoolVar(&jsonOutput, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("show does not accept positional arguments")
	}
	if id == "" {
		return errors.New("show requires --id")
	}
	store := ledger.NewStore(path)
	current, err := store.Load()
	if err != nil {
		return err
	}
	target, exists := current.Find(id)
	if !exists {
		return fmt.Errorf("survey %q not found", id)
	}
	if jsonOutput {
		return encodeJSON(stdout, target)
	}
	printSurvey(stdout, target)
	return nil
}

func runSmoke(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("smoke", stderr)
	var jsonOutput bool
	fs.BoolVar(&jsonOutput, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("smoke does not accept positional arguments")
	}
	directory, err := os.MkdirTemp("", "spokespan-smoke-")
	if err != nil {
		return fmt.Errorf("create smoke workspace: %w", err)
	}
	defer os.RemoveAll(directory)

	store := ledger.NewStore(filepath.Join(directory, "ledger.json"))
	current, err := store.Load()
	if err != nil {
		return err
	}
	label := "community workshop rear wheel"
	created, err := survey.New("smoke-survey", 4, 90, 110, 8, &label)
	if err != nil {
		return err
	}
	if err := current.Add(created); err != nil {
		return err
	}
	if err := store.Save(current); err != nil {
		return err
	}
	readings := []survey.Reading{{Spoke: 1, Tension: 98}, {Spoke: 2, Tension: 101}, {Spoke: 3, Tension: 99}, {Spoke: 4, Tension: 100}}
	for _, reading := range readings {
		current, err = store.Load()
		if err != nil {
			return err
		}
		target, exists := current.Find(created.ID)
		if !exists {
			return errors.New("smoke survey disappeared")
		}
		if err := target.Measure(reading.Spoke, reading.Tension); err != nil {
			return err
		}
		if err := store.Save(current); err != nil {
			return err
		}
	}
	current, err = store.Load()
	if err != nil {
		return err
	}
	target, exists := current.Find(created.ID)
	if !exists {
		return errors.New("smoke survey disappeared before finalization")
	}
	if err := target.Finalize(); err != nil {
		return err
	}
	if target.Status != survey.Balanced || target.Report == nil {
		return errors.New("smoke survey did not produce a balanced report")
	}
	if err := store.Save(current); err != nil {
		return err
	}
	current, err = store.Load()
	if err != nil {
		return err
	}
	target, exists = current.Find(created.ID)
	if !exists || target.Report == nil || target.Status != survey.Balanced {
		return errors.New("smoke report could not be reloaded")
	}
	if err := target.Measure(1, 100); err == nil {
		return errors.New("smoke allowed a measurement after finalization")
	}
	if jsonOutput {
		return encodeJSON(stdout, target)
	}
	fmt.Fprintf(stdout, "smoke ok: %s (%s)\n", target.ID, target.Status)
	return nil
}

func newUniqueID(current *ledger.Ledger) (string, error) {
	for attempts := 0; attempts < 5; attempts++ {
		id, err := survey.NewID()
		if err != nil {
			return "", err
		}
		if _, exists := current.Find(id); !exists {
			return id, nil
		}
	}
	return "", errors.New("could not create a unique survey ID")
}

func globalLedger(args []string) (string, []string, error) {
	path := defaultLedgerPath
	for len(args) > 0 {
		if args[0] == "--ledger" {
			if len(args) == 1 {
				return "", nil, errors.New("--ledger requires a path")
			}
			path = args[1]
			args = args[2:]
			continue
		}
		if strings.HasPrefix(args[0], "--ledger=") {
			path = strings.TrimPrefix(args[0], "--ledger=")
			if path == "" {
				return "", nil, errors.New("--ledger requires a path")
			}
			args = args[1:]
			continue
		}
		break
	}
	return path, args, nil
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func encodeJSON(w io.Writer, value any) error {
	return json.NewEncoder(w).Encode(value)
}

func printSurvey(w io.Writer, s *survey.Survey) {
	label := "(none)"
	if s.WheelLabel != nil {
		label = fmt.Sprintf("%q", *s.WheelLabel)
	}
	fmt.Fprintf(w, "Survey %s\n", s.ID)
	fmt.Fprintf(w, "Wheel label: %s\n", label)
	fmt.Fprintf(w, "Status: %s\n", s.Status)
	fmt.Fprintf(w, "Tension range: %.2f-%.2f; maximum spread: %.2f\n", s.MinTension, s.MaxTension, s.MaxSpread)
	fmt.Fprintf(w, "Readings: %d/%d\n", len(s.Readings), s.SpokeCount)
	for _, reading := range s.SortedReadings() {
		fmt.Fprintf(w, "  spoke %d: %.2f\n", reading.Spoke, reading.Tension)
	}
	if s.Report != nil {
		fmt.Fprintf(w, "Report: %s; observed %.2f-%.2f; spread %.2f\n", s.Report.Outcome, s.Report.MinimumTension, s.Report.MaximumTension, s.Report.Spread)
		fmt.Fprintf(w, "Range check: %t; spread check: %t\n", s.Report.WithinRange, s.Report.WithinMaxSpread)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "SpokeSpan records and evaluates bicycle-wheel spoke tension.")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  spokespan [--ledger PATH] start --spokes N --min-tension N --max-tension N --max-spread N [--label TEXT]")
	fmt.Fprintln(w, "  spokespan [--ledger PATH] measure --id ID --spoke N --tension N")
	fmt.Fprintln(w, "  spokespan [--ledger PATH] finalize --id ID")
	fmt.Fprintln(w, "  spokespan [--ledger PATH] show --id ID")
	fmt.Fprintln(w, "  spokespan smoke")
}

func writeError(w io.Writer, err error) int {
	fmt.Fprintf(w, "spokespan: %v\n", err)
	return 1
}

// Package survey contains the rules for recording and evaluating a wheel
// tensioning survey.
package survey

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Status describes the lifecycle state or final outcome of a survey.
type Status string

const (
	Tensioning Status = "tensioning"
	Balanced   Status = "balanced"
	Adjust     Status = "adjust"
)

// Reading is one measured spoke tension.
type Reading struct {
	Spoke   int     `json:"spoke"`
	Tension float64 `json:"tension"`
}

// Report is the immutable result produced when a complete survey is
// finalized.
type Report struct {
	Outcome         Status  `json:"outcome"`
	MinimumTension  float64 `json:"minimum_tension"`
	MaximumTension  float64 `json:"maximum_tension"`
	Spread          float64 `json:"spread"`
	WithinRange     bool    `json:"within_range"`
	WithinMaxSpread bool    `json:"within_max_spread"`
	OutOfRange      []int   `json:"out_of_range"`
}

// Survey records the configuration, readings, and final report for one wheel.
type Survey struct {
	ID         string    `json:"id"`
	WheelLabel *string   `json:"wheel_label,omitempty"`
	SpokeCount int       `json:"spoke_count"`
	MinTension float64   `json:"min_tension"`
	MaxTension float64   `json:"max_tension"`
	MaxSpread  float64   `json:"max_spread"`
	Status     Status    `json:"status"`
	Readings   []Reading `json:"readings"`
	Report     *Report   `json:"report,omitempty"`
}

// New creates a survey in the tensioning state.
func New(id string, spokeCount int, minTension, maxTension, maxSpread float64, wheelLabel *string) (Survey, error) {
	s := Survey{
		ID:         id,
		WheelLabel: wheelLabel,
		SpokeCount: spokeCount,
		MinTension: minTension,
		MaxTension: maxTension,
		MaxSpread:  maxSpread,
		Status:     Tensioning,
	}
	if err := s.validateConfiguration(); err != nil {
		return Survey{}, err
	}
	s.Readings = make([]Reading, 0, spokeCount)
	return s, nil
}

// Validate checks the persisted shape and lifecycle invariants.
func (s Survey) Validate() error {
	if s.ID == "" {
		return errors.New("survey id is required")
	}
	if err := s.validateConfiguration(); err != nil {
		return err
	}
	seen := make(map[int]struct{}, len(s.Readings))
	for _, reading := range s.Readings {
		if reading.Spoke < 1 || reading.Spoke > s.SpokeCount {
			return fmt.Errorf("survey %q has invalid spoke number %d", s.ID, reading.Spoke)
		}
		if !positiveFinite(reading.Tension) {
			return fmt.Errorf("survey %q has invalid tension for spoke %d", s.ID, reading.Spoke)
		}
		if _, exists := seen[reading.Spoke]; exists {
			return fmt.Errorf("survey %q has duplicate reading for spoke %d", s.ID, reading.Spoke)
		}
		seen[reading.Spoke] = struct{}{}
	}

	switch s.Status {
	case Tensioning:
		if s.Report != nil {
			return fmt.Errorf("tensioning survey %q cannot have a report", s.ID)
		}
	case Balanced, Adjust:
		if s.Report == nil {
			return fmt.Errorf("finalized survey %q is missing its report", s.ID)
		}
		if len(s.Readings) != s.SpokeCount {
			return fmt.Errorf("finalized survey %q is incomplete", s.ID)
		}
		if err := s.validateReport(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("survey %q has unsupported status %q", s.ID, s.Status)
	}
	return nil
}

// Measure records one spoke reading. Failed calls leave the survey unchanged.
func (s *Survey) Measure(spoke int, tension float64) error {
	if s.Status != Tensioning {
		return fmt.Errorf("survey %q is already finalized", s.ID)
	}
	if spoke < 1 || spoke > s.SpokeCount {
		return fmt.Errorf("spoke number must be between 1 and %d", s.SpokeCount)
	}
	if !positiveFinite(tension) {
		return errors.New("tension must be a positive finite number")
	}
	for _, reading := range s.Readings {
		if reading.Spoke == spoke {
			return fmt.Errorf("spoke %d already has a reading", spoke)
		}
	}
	s.Readings = append(s.Readings, Reading{Spoke: spoke, Tension: tension})
	return nil
}

// Finalize evaluates a complete survey and freezes its report.
func (s *Survey) Finalize() error {
	if s.Status != Tensioning {
		return fmt.Errorf("survey %q is already finalized", s.ID)
	}
	if len(s.Readings) != s.SpokeCount {
		return fmt.Errorf("survey %q needs readings for all %d spokes; have %d", s.ID, s.SpokeCount, len(s.Readings))
	}
	report := s.deriveReport()
	s.Report = &report
	s.Status = report.Outcome
	return nil
}

// SortedReadings returns a copy ordered by spoke number for stable display.
func (s Survey) SortedReadings() []Reading {
	readings := s.Readings
	sort.Slice(readings, func(i, j int) bool { return readings[i].Spoke < readings[j].Spoke })
	return readings
}

func (s Survey) validateConfiguration() error {
	if s.ID == "" {
		return errors.New("survey id is required")
	}
	if s.SpokeCount < 1 {
		return errors.New("spoke count must be at least 1")
	}
	if !positiveFinite(s.MinTension) || !positiveFinite(s.MaxTension) {
		return errors.New("tension range must contain positive finite numbers")
	}
	if s.MinTension >= s.MaxTension {
		return errors.New("minimum tension must be less than maximum tension")
	}
	if math.IsNaN(s.MaxSpread) || math.IsInf(s.MaxSpread, 0) || s.MaxSpread < 0 {
		return errors.New("maximum spread must be a finite non-negative number")
	}
	return nil
}

func (s Survey) validateReport() error {
	if s.Report.Outcome != s.Status {
		return fmt.Errorf("survey %q report outcome does not match status", s.ID)
	}
	if !positiveFinite(s.Report.MinimumTension) || !positiveFinite(s.Report.MaximumTension) || s.Report.MinimumTension > s.Report.MaximumTension {
		return fmt.Errorf("survey %q has invalid report range", s.ID)
	}
	if math.IsNaN(s.Report.Spread) || math.IsInf(s.Report.Spread, 0) || s.Report.Spread < 0 {
		return fmt.Errorf("survey %q has invalid report spread", s.ID)
	}
	derived := s.deriveReport()
	if derived.Outcome != s.Report.Outcome || derived.MinimumTension != s.Report.MinimumTension || derived.MaximumTension != s.Report.MaximumTension || derived.Spread != s.Report.Spread || derived.WithinRange != s.Report.WithinRange || derived.WithinMaxSpread != s.Report.WithinMaxSpread || !sameInts(derived.OutOfRange, s.Report.OutOfRange) {
		return fmt.Errorf("survey %q report does not match its readings", s.ID)
	}
	return nil
}

func (s Survey) deriveReport() Report {
	readings := s.SortedReadings()
	minimum := readings[0].Tension
	maximum := readings[0].Tension
	outOfRange := make([]int, 0)
	for _, reading := range readings {
		if reading.Tension < minimum {
			minimum = reading.Tension
		}
		if reading.Tension > maximum {
			maximum = reading.Tension
		}
		if reading.Tension < s.MinTension || reading.Tension > s.MaxTension {
			outOfRange = append(outOfRange, reading.Spoke)
		}
	}
	spread := maximum - minimum
	withinRange := len(outOfRange) == 0
	withinMaxSpread := spread <= s.MaxSpread
	outcome := Balanced
	if !withinRange || !withinMaxSpread {
		outcome = Adjust
	}
	return Report{
		Outcome:         outcome,
		MinimumTension:  minimum,
		MaximumTension:  maximum,
		Spread:          spread,
		WithinRange:     withinRange,
		WithinMaxSpread: withinMaxSpread,
		OutOfRange:      outOfRange,
	}
}

func positiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func sameInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

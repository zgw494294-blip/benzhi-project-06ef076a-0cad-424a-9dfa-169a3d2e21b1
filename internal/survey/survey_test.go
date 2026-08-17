package survey

import (
	"reflect"
	"strings"
	"testing"
)

func testSurvey(t *testing.T, maxSpread float64) Survey {
	t.Helper()
	s, err := New("wheel-1", 3, 90, 110, maxSpread, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	cases := []struct {
		name string
		args func() (Survey, error)
	}{
		{name: "negative spoke count", args: func() (Survey, error) { return New("x", -1, 90, 110, 5, nil) }},
		{name: "reversed range", args: func() (Survey, error) { return New("x", 1, 110, 90, 5, nil) }},
		{name: "negative spread", args: func() (Survey, error) { return New("x", 1, 90, 110, -1, nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.args(); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func TestMeasureRejectsInvalidReadingsWithoutMutation(t *testing.T) {
	s := testSurvey(t, 5)
	if err := s.Measure(1, 100); err != nil {
		t.Fatal(err)
	}
	before := append([]Reading(nil), s.Readings...)
	for _, tc := range []struct {
		name    string
		spoke   int
		tension float64
		want    string
	}{
		{name: "duplicate", spoke: 1, tension: 101, want: "already has"},
		{name: "zero spoke", spoke: 0, tension: 101, want: "between"},
		{name: "too large spoke", spoke: 4, tension: 101, want: "between"},
		{name: "zero tension", spoke: 2, tension: 0, want: "positive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.Measure(tc.spoke, tc.tension); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			if !reflect.DeepEqual(before, s.Readings) {
				t.Fatal("failed measurement changed stored readings")
			}
		})
	}
}

func TestFinalizeProducesBalancedReportAndFreezesSurvey(t *testing.T) {
	s, err := New("wheel-3", 4, 90, 110, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, reading := range []Reading{{Spoke: 3, Tension: 101}, {Spoke: 1, Tension: 98}, {Spoke: 2, Tension: 100}} {
		if err := s.Measure(reading.Spoke, reading.Tension); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Finalize(); err == nil {
		t.Fatal("expected incomplete survey error")
	}
	if err := s.Measure(4, 99); err != nil {
		t.Fatal(err)
	}
	if err := s.Finalize(); err != nil {
		t.Fatal(err)
	}
	if s.Status != Balanced || s.Report == nil {
		t.Fatalf("expected balanced report, got status=%q report=%v", s.Status, s.Report)
	}
	if s.Report.MinimumTension != 98 || s.Report.MaximumTension != 101 || s.Report.Spread != 3 || !s.Report.WithinRange || !s.Report.WithinMaxSpread {
		t.Fatalf("unexpected report: %#v", s.Report)
	}
	snapshot := *s.Report
	if err := s.Measure(5, 100); err == nil {
		t.Fatal("expected measurement after finalization to fail")
	}
	if err := s.Finalize(); err == nil {
		t.Fatal("expected repeated finalization to fail")
	}
	if !reflect.DeepEqual(snapshot, *s.Report) {
		t.Fatal("final report changed after rejected operations")
	}
}

func TestFinalizeMarksOutOfRangeOrWideSurveyForAdjustment(t *testing.T) {
	s, err := New("wheel-2", 3, 90, 110, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, reading := range []Reading{{Spoke: 1, Tension: 89}, {Spoke: 2, Tension: 100}, {Spoke: 3, Tension: 101}} {
		if err := s.Measure(reading.Spoke, reading.Tension); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Finalize(); err != nil {
		t.Fatal(err)
	}
	if s.Status != Adjust || s.Report.WithinRange || s.Report.WithinMaxSpread || !reflect.DeepEqual(s.Report.OutOfRange, []int{1}) {
		t.Fatalf("unexpected adjustment report: %#v", s.Report)
	}
}

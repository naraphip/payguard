package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBookingShadowWindowValidation(t *testing.T) {
	now := time.Date(2026, 11, 21, 5, 0, 0, 0, time.UTC)
	valid := bookingShadowWindow{
		HoldoutStart: now.Add(-time.Hour), HoldoutEnd: now.Add(-time.Minute),
		AsOf: now, ReviewThreshold: 5,
	}
	if !valid.valid(now) {
		t.Fatal("a chronological holdout ending by the as-of time should be accepted")
	}
	tests := []struct {
		name string
		edit func(*bookingShadowWindow)
	}{
		{"empty interval", func(w *bookingShadowWindow) { w.HoldoutEnd = w.HoldoutStart }},
		{"cutoff before holdout", func(w *bookingShadowWindow) { w.AsOf = w.HoldoutStart }},
		{"future cutoff", func(w *bookingShadowWindow) { w.AsOf = now.Add(time.Microsecond) }},
		{"sub-microsecond boundary", func(w *bookingShadowWindow) { w.HoldoutStart = w.HoldoutStart.Add(time.Nanosecond) }},
		{"non-positive threshold", func(w *bookingShadowWindow) { w.ReviewThreshold = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.edit(&input)
			if input.valid(now) {
				t.Fatal("accepted invalid input")
			}
		})
	}
}

func TestBookingShadowUsesPinnedEvidence(t *testing.T) {
	cutoff := time.Date(2026, 11, 21, 5, 0, 0, 0, time.UTC)
	report := bookingShadowReport{Window: bookingShadowWindow{AsOf: cutoff, ReviewThreshold: 3}}
	profile := profileCandidate{SchemaVersion: 1, Stage: bookingStage, Signals: []string{bookingTimestamp}}
	event := bookingEvent{bookingInput: bookingInput{OccurredAt: cutoff.Add(-time.Hour)}}
	assessedAt := cutoff.Add(-time.Minute)
	version := "baseline-v1-1"
	for _, fixture := range []struct {
		velocity int64
		label    string
	}{
		{6, "fraud"}, {4, "fraud"}, {4, "normal"}, {1, "normal"},
	} {
		assessment := evaluateBooking(event, "profile-v1", profile, fixture.velocity, bookingJevMock)
		assessment.AssessedAt = &assessedAt
		assessment.Baseline = bookingBaseline{State: "ACTIVE", Version: &version}
		includeBookingShadowFixture(t, &report, assessment, fixture.label, 1, true)
	}
	report.Stored.finish()
	report.Shadow.finish()
	if report.Bookings != 4 || len(report.Lineage) != 1 || *report.Lineage[0].BaselineVersion != version {
		t.Fatalf("unexpected report bookings or lineage: %+v", report)
	}
	if report.Stored.TP != 1 || report.Stored.TN != 2 || report.Stored.FP != 0 || report.Stored.FN != 1 ||
		report.Stored.FPR == nil || *report.Stored.FPR != 0 || report.Stored.Recall == nil || *report.Stored.Recall != 0.5 {
		t.Fatalf("wrong stored confusion matrix: %+v", report.Stored)
	}
	if report.Shadow.TP != 2 || report.Shadow.TN != 1 || report.Shadow.FP != 1 || report.Shadow.FN != 0 ||
		report.Shadow.FPR == nil || *report.Shadow.FPR != 0.5 || report.Shadow.Recall == nil || *report.Shadow.Recall != 1 {
		t.Fatalf("wrong shadow confusion matrix: %+v", report.Shadow)
	}
}

func TestBookingShadowExcludesUnscorableSamples(t *testing.T) {
	cutoff := time.Date(2026, 11, 21, 5, 0, 0, 0, time.UTC)
	report := bookingShadowReport{Window: bookingShadowWindow{AsOf: cutoff, ReviewThreshold: 3}}
	assessment := evaluateBooking(bookingEvent{}, "profile-v1", profileCandidate{}, 1, bookingJevMock)
	future := cutoff.Add(time.Minute)
	assessment.AssessedAt = &future
	includeBookingShadowFixture(t, &report, assessment, "fraud", 1, true)
	assessment.AssessedAt = &cutoff
	includeBookingShadowFixture(t, &report, assessment, "fraud", 1, false)
	includeBookingShadowFixture(t, &report, assessment, "fraud", 2, true)
	includeBookingShadowFixture(t, &report, assessment, "", 0, true)
	assessment.Outcome = "UNRESOLVED"
	includeBookingShadowFixture(t, &report, assessment, "fraud", 1, true)
	assessment.Outcome = "LOW_RISK"
	assessment.Features = nil
	includeBookingShadowFixture(t, &report, assessment, "normal", 1, true)
	report.Stored.finish()
	report.Shadow.finish()
	expected := bookingShadowExcluded{AssessmentUnavailable: 1, LabelUnavailable: 1, AmbiguousLabel: 1, BaselineOverlap: 1, Unresolved: 1, EvidenceUnavailable: 1}
	if report.Bookings != 6 || report.Excluded != expected || len(report.Lineage) != 0 {
		t.Fatalf("wrong exclusions: %+v", report)
	}
	if report.Stored.FPR != nil || report.Stored.Recall != nil || report.Shadow.FPR != nil || report.Shadow.Recall != nil {
		t.Fatal("undefined metrics must be null")
	}
}

func includeBookingShadowFixture(t *testing.T, report *bookingShadowReport, assessment bookingAssessment, label string, count int, baselineValid bool) {
	t.Helper()
	raw, err := json.Marshal(assessment)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.include(bookingShadowSample{raw: raw, label: label, labelCount: count, baselineValid: baselineValid}); err != nil {
		t.Fatal(err)
	}
}

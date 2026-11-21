package server

import (
	"testing"
	"time"
)

func TestActivityRangeAvailability(t *testing.T) {
	today := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC)
	month := findActivityRange("1m")
	if start := month.first(today).Format(time.DateOnly); start != "2026-03-01" {
		t.Fatalf("one month started on %s", start)
	}
	chart := makeActivityChart(today, findActivityRange("invalid"), "2026-03-20", nil, nil)
	if chart.Grain != "day" || len(chart.Dates) != 28 {
		t.Fatalf("default chart has %d %s columns", len(chart.Dates), chart.Grain)
	}
	for _, option := range chart.Options {
		if option.Selected != (option.Key == "4w") || option.Disabled != (option.Key != "4w" && option.Key != "2w" && option.Key != "1w") {
			t.Fatalf("unexpected range option: %+v", option)
		}
	}
	year := makeActivityChart(today, findActivityRange("1y"), "2024-01-01", nil, nil)
	if year.Grain != "week" || len(year.Dates) != 53 || year.Dates[0].Date != "2025-04-01" {
		t.Fatalf("unexpected yearly chart: %d %s columns", len(year.Dates), year.Grain)
	}
}

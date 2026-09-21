package v0

import (
	"testing"
	"time"
)

func TestGetAge_RoundingUnit(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		age  time.Duration
		unit time.Duration
	}{
		{"<hour rounds to second", 5*time.Minute + 12*time.Second, time.Second},
		{"<day rounds to minute", 2*time.Hour + 3*time.Minute, time.Minute},
		{"<week rounds to hour", 3*24*time.Hour + 5*time.Hour, time.Hour},
		{"<month rounds to day", 2*7*24*time.Hour + 3*24*time.Hour, 24 * time.Hour},
		{"<year rounds to week", 60 * 24 * time.Hour, 7 * 24 * time.Hour},
		{">=year rounds to month", 400 * 24 * time.Hour, 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := now.Add(-tt.age)
			got := *GetAge(&ts)
			if got%tt.unit != 0 {
				t.Fatalf("age=%v got=%v; expected multiple of %v", tt.age, got, tt.unit)
			}
		})
	}
}

func TestGetAgeFormattedPrecise(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		age  time.Duration
		want string
	}{
		// pick ages away from rounding boundaries; scheduler skew adds a few ms
		{"sub-second rounds to 100ms", 380 * time.Millisecond, "400ms"},
		{"under-minute rounds to second", 42 * time.Second, "42s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// construct a timestamp of the fixture age
			ts := now.Add(-tt.age)
			// check the formatted precise age
			if got := GetAgeFormattedPrecise(&ts); got != tt.want {
				t.Fatalf("GetAgeFormattedPrecise(age=%v) = %q, want %q", tt.age, got, tt.want)
			}
		})
	}
}

func TestGetAgeFormatted(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		age  time.Duration
		want string
	}{
		{"zero renders as 0s", 0, "0s"},
		{"sub-minute renders raw seconds", 45 * time.Second, "45s"},
		{"whole minute drops trailing 0s", 60 * time.Second, "1m"},
		{"minute plus seconds keeps both", 65 * time.Second, "1m5s"},
		{"<hour keeps seconds", 5*time.Minute + 23*time.Second, "5m23s"},
		{"hour plus minutes unchanged", 1*time.Hour + 5*time.Minute, "1h5m"},
		{"hours only when minutes=0", 2 * time.Hour, "2h"},
		{"hours+minutes when minutes>0", 2*time.Hour + 15*time.Minute, "2h15m"},
		{"days only when hours=0", 3 * 24 * time.Hour, "3d"},
		{"weeks only when days=0", 2 * 7 * 24 * time.Hour, "2w"},
		// 60d rounds to 63d (= 9w) => 2mo1w per current logic.
		{"months+weeks (<year)", 60 * 24 * time.Hour, "2mo1w"},
		// 730d rounds to 720d (= 24mo) => 1y per current logic.
		{"years only when months=0", 2 * 365 * 24 * time.Hour, "1y"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := now.Add(-tt.age)
			if got := GetAgeFormatted(&ts); got != tt.want {
				t.Fatalf("GetAgeFormatted(age=%v) = %q, want %q", tt.age, got, tt.want)
			}
		})
	}
}

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

func TestGetAgeFormatted(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		age  time.Duration
		want string
	}{
		{"<hour keeps seconds", 5*time.Minute + 23*time.Second, "5m23s"},
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


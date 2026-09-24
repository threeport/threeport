package v0

import (
	"fmt"
	"time"
)

const (
	minute = time.Minute
	hour   = time.Hour
	day    = 24 * time.Hour
	week   = 7 * day
	month  = 30 * day
	year   = 365 * day
)

// GetAge returns the age of a timestamp, rounded to an appropriate unit based on duration.
// Rounding rules:
// - Less than 1 minute: round to nearest second
// - 1 minute to 1 hour: round to nearest second
// - 1 hour to 24 hours: round to nearest minute
// - 24 hours to 1 week: round to nearest hour
// - 1 week to 1 month (30 days): round to nearest day
// - 1 month to 1 year: round to nearest week
// - More than 1 year: round to nearest month (30 days)
func GetAge(timestamp *time.Time) *time.Duration {
	now := time.Now()
	duration := now.Sub(*timestamp)

	var roundedTime time.Duration

	switch {
	case duration < hour:
		// Less than 1 hour: round to nearest second
		roundedTime = duration.Round(time.Second)
	case duration < day:
		// 1 hour to 24 hours: round to nearest minute
		roundedTime = duration.Round(minute)
	case duration < week:
		// 24 hours to 1 week: round to nearest hour
		roundedTime = duration.Round(hour)
	case duration < month:
		// 1 week to 1 month: round to nearest day
		roundedTime = duration.Round(day)
	case duration < year:
		// 1 month to 1 year: round to nearest week
		roundedTime = duration.Round(week)
	default:
		// More than 1 year: round to nearest month (30 days)
		roundedTime = duration.Round(month)
	}

	return &roundedTime
}

// GetAgeFormatted returns the age of a timestamp as a formatted string,
// rounded and displayed at an appropriate precision based on duration.
// Uses GetAge for consistent rounding logic, then formats appropriately.
// Examples: "42s", "5m23s", "2h15m", "3d", "2w", "4mo"
func GetAgeFormatted(timestamp *time.Time) string {
	// Use GetAge to get the properly rounded duration
	roundedDuration := GetAge(timestamp)
	duration := *roundedDuration

	switch {
	case duration < hour:
		// Less than 1 hour: show full precision (already rounded to seconds)
		return duration.String()
	case duration < day:
		// 1 hour to 24 hours: show hours and minutes only (rounded to minutes)
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, minutes)
	case duration < week:
		// 24 hours to 1 week: show days and hours (rounded to hours)
		days := int(duration.Hours()) / 24
		hours := int(duration.Hours()) % 24
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd%dh", days, hours)
	case duration < month:
		// 1 week to 1 month: show weeks and days (rounded to days)
		weeks := int(duration.Hours()) / (24 * 7)
		days := (int(duration.Hours()) / 24) % 7
		if days == 0 {
			return fmt.Sprintf("%dw", weeks)
		}
		return fmt.Sprintf("%dw%dd", weeks, days)
	case duration < year:
		// 1 month to 1 year: show months and weeks (rounded to weeks)
		months := int(duration.Hours()) / (24 * 30)
		weeks := (int(duration.Hours()) / (24 * 7)) % 4
		if weeks == 0 {
			return fmt.Sprintf("%dmo", months)
		}
		return fmt.Sprintf("%dmo%dw", months, weeks)
	default:
		// More than 1 year: show years and months (rounded to months)
		years := int(duration.Hours()) / (24 * 365)
		months := (int(duration.Hours()) / (24 * 30)) % 12
		if months == 0 {
			return fmt.Sprintf("%dy", years)
		}
		return fmt.Sprintf("%dy%dmo", years, months)
	}
}

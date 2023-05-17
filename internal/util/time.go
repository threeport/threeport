package util

import (
	"math"
	"time"
)

// GetAge returns the age of a timestamp, rounded to the nearest second.
func GetAge(timestamp *time.Time) *time.Duration {
	now := time.Now()
	duration := now.Sub(*timestamp)
	floatDuration := duration.Seconds()
	roundedDuration := math.Round(floatDuration)
	roundedTime := time.Duration(roundedDuration * 1e9)

	return &roundedTime
}

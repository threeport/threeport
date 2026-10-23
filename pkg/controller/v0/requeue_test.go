package controller

import (
	"testing"
	"time"
)

func TestRetriesExhausted(t *testing.T) {
	ago := func(d time.Duration) *int64 {
		v := time.Now().Add(-d).Unix()
		return &v
	}

	tests := []struct {
		name         string
		creationTime *int64
		want         bool
	}{
		{"no creation time is never exhausted", nil, false},
		{"fresh notification", ago(time.Second), false},
		{"within the bound", ago(DefaultMaxReconcileDuration - time.Minute), false},
		{"past the bound", ago(DefaultMaxReconcileDuration + time.Minute), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RetriesExhausted(tt.creationTime); got != tt.want {
				t.Errorf("RetriesExhausted() = %v, want %v", got, tt.want)
			}
		})
	}
}

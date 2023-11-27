package v0

import (
	"strings"

	"go.uber.org/zap/zapcore"
)

// SensitiveStrings provides the sensitive string values that should be
// suppressed from log output.
func SensitiveStrings() []string {
	return []string{
		"PRIVATE KEY",
	}
}

// SuppressSensitiveCore allows us to suppress sensitive strings.
type SuppressSensitiveCore struct {
	zapcore.Core
	sensitiveStrings []string
}

// Write suppresses sensitive strings from log output.
func (s *SuppressSensitiveCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	// check if the message contains any of the sensitive strings
	for _, str := range s.sensitiveStrings {
		if strings.Contains(entry.Message, str) {
			return nil // suppress the log message
		}
	}

	// otherwise, write the log message using the parent core
	return s.Core.Write(entry, fields)
}

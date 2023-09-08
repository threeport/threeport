package log

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger returns a new zap logger for development or production purposes
// based on whether verbose logging was requested with the -verbose flag.
func NewLogger(verbose bool) (zap.Logger, error) {
	var logger zap.Logger
	switch verbose {
	case true:
		zapLog, err := zap.NewDevelopment(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return &SuppressSensitiveCore{
				Core:             core,
				sensitiveStrings: SensitiveStrings(),
			}
		}))
		if err != nil {
			return logger, fmt.Errorf("failed to set up development logging: %w", err)
		}
		logger = *zapLog
	default:
		zapLog, err := zap.NewProduction(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return &SuppressSensitiveCore{
				Core:             core,
				sensitiveStrings: SensitiveStrings(),
			}
		}))
		if err != nil {
			return logger, fmt.Errorf("failed to set up production logging: %v", err)
		}
		logger = *zapLog
	}

	return logger, nil
}

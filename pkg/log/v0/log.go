package v0

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger returns a new zap logger for development or production purposes
// based on whether verbose logging was requested with the -verbose flag.
func NewLogger(verbose bool) (zap.Logger, error) {
	var logger zap.Logger

	// define a custom time encoder to output timestamps as "2006-01-02 15:04:05.000000000"
	customTimeEncoder := func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05.000000000"))
	}

	// pick a zap configuration based on the verbose flag and insert the custom time encoder
	switch verbose {
	case true:
		cfg := zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeTime = customTimeEncoder
		zapLog, err := cfg.Build(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
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
		cfg := zap.NewProductionConfig()
		cfg.EncoderConfig.EncodeTime = customTimeEncoder
		zapLog, err := cfg.Build(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
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

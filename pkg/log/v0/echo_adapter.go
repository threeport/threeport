package v0

import (
	"fmt"
	"io"

	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"go.uber.org/zap"
)

// EchoLogger adapts zap.Logger to echo.Logger.
type EchoLogger struct {
	logger *zap.Logger
	level  log.Lvl
	prefix string
	header string
}

// NewEchoLogger creates a new Echo logger backed by Zap.
func NewEchoLogger(l *zap.Logger) echo.Logger {
	return &EchoLogger{
		logger: l,
		// Set default log level as INFO.
		level: log.INFO,
	}
}

// Output returns the output destination for the logger.
// Not supported with zap; returning nil.
func (l *EchoLogger) Output() io.Writer {
	return nil
}

// SetOutput is a no-op for Zap (since zap does not use an io.Writer).
func (l *EchoLogger) SetOutput(w io.Writer) {
	// no-op
}

// Prefix returns the current prefix.
func (l *EchoLogger) Prefix() string {
	return l.prefix
}

// SetPrefix sets the logging prefix.
func (l *EchoLogger) SetPrefix(p string) {
	l.prefix = p
}

// Level returns the current logging level.
func (l *EchoLogger) Level() log.Lvl {
	return l.level
}

// SetLevel sets the logger level.
func (l *EchoLogger) SetLevel(v log.Lvl) {
	l.level = v
}

// SetHeader sets the header for log output (not used by zap).
func (l *EchoLogger) SetHeader(h string) {
	l.header = h
}

// Print logs at INFO level.
func (l *EchoLogger) Print(i ...interface{}) {
	l.logger.Info(fmt.Sprint(i...))
}

// Printf logs at INFO level.
func (l *EchoLogger) Printf(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

// Printj logs JSON input at INFO level.
func (l *EchoLogger) Printj(j log.JSON) {
	l.logger.Info(fmt.Sprint(j))
}

// Debug logs at DEBUG level.
func (l *EchoLogger) Debug(i ...interface{}) {
	l.logger.Debug(fmt.Sprint(i...))
}

// Debugf logs at DEBUG level.
func (l *EchoLogger) Debugf(format string, args ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, args...))
}

// Debugj logs JSON input at DEBUG level.
func (l *EchoLogger) Debugj(j log.JSON) {
	l.logger.Debug(fmt.Sprint(j))
}

// Info logs at INFO level.
func (l *EchoLogger) Info(i ...interface{}) {
	l.logger.Info(fmt.Sprint(i...))
}

// Infof logs at INFO level.
func (l *EchoLogger) Infof(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

// Infoj logs JSON input at INFO level.
func (l *EchoLogger) Infoj(j log.JSON) {
	l.logger.Info(fmt.Sprint(j))
}

// Warn logs at WARN level.
func (l *EchoLogger) Warn(i ...interface{}) {
	l.logger.Warn(fmt.Sprint(i...))
}

// Warnf logs at WARN level.
func (l *EchoLogger) Warnf(format string, args ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}

// Warnj logs JSON input at WARN level.
func (l *EchoLogger) Warnj(j log.JSON) {
	l.logger.Warn(fmt.Sprint(j))
}

// Error logs at ERROR level.
func (l *EchoLogger) Error(i ...interface{}) {
	l.logger.Error(fmt.Sprint(i...))
}

// Errorf logs at ERROR level.
func (l *EchoLogger) Errorf(format string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, args...))
}

// Errorj logs JSON input at ERROR level.
func (l *EchoLogger) Errorj(j log.JSON) {
	l.logger.Error(fmt.Sprint(j))
}

// Fatal logs at FATAL level.
func (l *EchoLogger) Fatal(i ...interface{}) {
	l.logger.Fatal(fmt.Sprint(i...))
}

// Fatalf logs at FATAL level.
func (l *EchoLogger) Fatalf(format string, args ...interface{}) {
	l.logger.Fatal(fmt.Sprintf(format, args...))
}

// Fatalj logs JSON input at FATAL level.
func (l *EchoLogger) Fatalj(j log.JSON) {
	l.logger.Fatal(fmt.Sprint(j))
}

// Panic logs at PANIC level.
func (l *EchoLogger) Panic(i ...interface{}) {
	l.logger.Panic(fmt.Sprint(i...))
}

// Panicf logs at PANIC level.
func (l *EchoLogger) Panicf(format string, args ...interface{}) {
	l.logger.Panic(fmt.Sprintf(format, args...))
}

// Panicj logs JSON input at PANIC level.
func (l *EchoLogger) Panicj(j log.JSON) {
	l.logger.Panic(fmt.Sprint(j))
}

// generated by 'threeport-codegen api-version' - do not edit

package database

import (
	"context"
	"fmt"
	log "github.com/threeport/threeport/internal/log"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	zap "go.uber.org/zap"
	postgres "gorm.io/driver/postgres"
	gorm "gorm.io/gorm"
	logger "gorm.io/gorm/logger"
	"os"
	"reflect"
	"strings"
	"time"
)

// ZapLogger is a custom GORM logger that forwards log messages to a Zap logger.
type ZapLogger struct {
	Logger *zap.Logger
}

// Init initializes the API database.
func Init(autoMigrate bool, logger *zap.Logger) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_SSL_MODE"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: &ZapLogger{Logger: logger},
		NowFunc: func() time.Time {
			utc, _ := time.LoadLocation("UTC")
			return time.Now().In(utc).Truncate(time.Microsecond)
		},
	})
	if err != nil {
		return nil, err
	}

	if autoMigrate {
<<<<<<< HEAD
		if err := db.AutoMigrate(
			&v0.Profile{},
			&v0.Tier{},
			&v0.AwsAccount{},
			&v0.AwsEksKubernetesRuntimeDefinition{},
			&v0.AwsEksKubernetesRuntimeInstance{},
			&v0.AwsRelationalDatabaseDefinition{},
			&v0.AwsRelationalDatabaseInstance{},
			&v0.Definition{},
			&v0.Instance{},
			&v0.DomainNameDefinition{},
			&v0.DomainNameInstance{},
			&v0.ForwardProxyDefinition{},
			&v0.ForwardProxyInstance{},
			&v0.GatewayDefinition{},
			&v0.GatewayInstance{},
			&v0.KubernetesRuntimeDefinition{},
			&v0.KubernetesRuntimeInstance{},
			&v0.LogBackend{},
			&v0.LogStorageDefinition{},
			&v0.LogStorageInstance{},
			&v0.WorkloadDefinition{},
			&v0.WorkloadResourceDefinition{},
			&v0.WorkloadInstance{},
			&v0.AttachedObjectReference{},
			&v0.WorkloadResourceInstance{},
			&v0.WorkloadEvent{},
		); err != nil {
			return nil, err
		}
=======
		db.AutoMigrate(&v0.Profile{})
		db.AutoMigrate(&v0.Tier{})
		db.AutoMigrate(&v0.AwsAccount{})
		db.AutoMigrate(&v0.AwsEksKubernetesRuntimeDefinition{})
		db.AutoMigrate(&v0.AwsEksKubernetesRuntimeInstance{})
		db.AutoMigrate(&v0.AwsRelationalDatabaseDefinition{})
		db.AutoMigrate(&v0.AwsRelationalDatabaseInstance{})
		db.AutoMigrate(&v0.Definition{})
		db.AutoMigrate(&v0.Instance{})
		db.AutoMigrate(&v0.DomainNameDefinition{})
		db.AutoMigrate(&v0.DomainNameInstance{})
		db.AutoMigrate(&v0.ForwardProxyDefinition{})
		db.AutoMigrate(&v0.ForwardProxyInstance{})
		db.AutoMigrate(&v0.KubernetesRuntimeDefinition{})
		db.AutoMigrate(&v0.KubernetesRuntimeInstance{})
		db.AutoMigrate(&v0.LogBackend{})
		db.AutoMigrate(&v0.LogStorageDefinition{})
		db.AutoMigrate(&v0.LogStorageInstance{})
		db.AutoMigrate(&v0.NetworkIngressDefinition{})
		db.AutoMigrate(&v0.NetworkIngressInstance{})
		db.AutoMigrate(&v0.WorkloadDefinition{})
		db.AutoMigrate(&v0.WorkloadResourceDefinition{})
		db.AutoMigrate(&v0.WorkloadInstance{})
		db.AutoMigrate(&v0.WorkloadResourceInstance{})
		db.AutoMigrate(&v0.WorkloadEvent{})

>>>>>>> c0a22ac (refactor: change cluster object name to kubernetes runtime)
	}

	return db, nil
}

// LogMode overrides the standard GORM logger's LogMode method to set the logger mode.
func (zl *ZapLogger) LogMode(level logger.LogLevel) logger.Interface {
	return zl
}

// Info overrides the standard GORM logger's Info method to forward log messages
// to the zap logger.
func (zl *ZapLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	fields := make([]zap.Field, 0, len(data))
	for i := 0; i < len(data); i += 2 {
		fields = append(fields, zap.Any(data[i].(string), data[i+1]))
	}
	zl.Logger.Info(msg, fields...)
}

// Warn overrides the standard GORM logger's Warn method to forward log messages
// to the zap logger.
func (zl *ZapLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	fields := make([]zap.Field, 0, len(data))
	for i := 0; i < len(data); i += 2 {
		fields = append(fields, zap.Any(data[i].(string), data[i+1]))
	}
	zl.Logger.Warn(msg, fields...)
}

// Error overrides the standard GORM logger's Error method to forward log messages
// to the zap logger.
func (zl *ZapLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	fields := make([]zap.Field, 0, len(data))
	for i := 0; i < len(data); i += 2 {
		if reflect.TypeOf(data[i]).Kind() == reflect.Ptr {
			data[i] = fmt.Sprintf("%+v", data[i])
		}
		fields = append(fields, zap.Any(data[i].(string), data[i+1]))
	}
	zl.Logger.Error(msg, fields...)
}

// Trace overrides the standard GORM logger's Trace method to forward log messages
// to the zap logger.
func (zl *ZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	// use the fc function to get the SQL statement and execution time
	sql, rows := fc()

	// create a new logger with some additional fields
	logger := zl.Logger.With(
		zap.String("type", "sql"),
		zap.String("sql", suppressSensitive(sql)),
		zap.Int64("rows", rows),
		zap.Duration("elapsed", time.Since(begin)),
	)

	// if an error occurred, add it as a field to the logger
	if err != nil {
		logger = logger.With(zap.Error(err))
	}

	// log the message using the logger
	logger.Debug("gorm query")
}

// suppressSensitive supresses messages containing sesitive strings.
func suppressSensitive(msg string) string {
	for _, str := range log.SensitiveStrings() {
		if strings.Contains(msg, str) {
			return fmt.Sprintf("[log message containing %s supporessed]", str)
		}
	}

	return msg
}

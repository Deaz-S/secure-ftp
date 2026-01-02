// Package logger provides structured logging with rotation support.
package logger

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	instance *Logger
	once     sync.Once
)

// Logger wraps zap logger with additional functionality.
type Logger struct {
	zapLogger *zap.Logger
	sugar     *zap.SugaredLogger
	logFile   *os.File
	logPath   string
	level     zapcore.Level
}

// Config holds logger configuration.
type Config struct {
	LogPath    string        // Path to log file
	Level      string        // Log level: debug, info, warn, error
	MaxSize    int64         // Max size in bytes before rotation (default 10MB)
	MaxBackups int           // Max number of backup files to keep
	Console    bool          // Also output to console
}

// GetInstance returns the singleton logger instance.
func GetInstance() *Logger {
	once.Do(func() {
		instance = &Logger{}
	})
	return instance
}

// Initialize sets up the logger with the given configuration.
func (l *Logger) Initialize(config Config) error {
	// Set default values
	if config.MaxSize == 0 {
		config.MaxSize = 10 * 1024 * 1024 // 10MB
	}
	if config.MaxBackups == 0 {
		config.MaxBackups = 5
	}

	// Parse log level
	l.level = zapcore.InfoLevel
	switch config.Level {
	case "debug":
		l.level = zapcore.DebugLevel
	case "info":
		l.level = zapcore.InfoLevel
	case "warn":
		l.level = zapcore.WarnLevel
	case "error":
		l.level = zapcore.ErrorLevel
	}

	// Create log directory if needed
	if config.LogPath != "" {
		dir := filepath.Dir(config.LogPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		// Open log file
		file, err := os.OpenFile(config.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		l.logFile = file
		l.logPath = config.LogPath
	}

	// Create encoder config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Build cores
	var cores []zapcore.Core

	// File core (JSON)
	if l.logFile != nil {
		fileEncoder := zapcore.NewJSONEncoder(encoderConfig)
		fileCore := zapcore.NewCore(fileEncoder, zapcore.AddSync(l.logFile), l.level)
		cores = append(cores, fileCore)
	}

	// Console core
	if config.Console {
		consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
		consoleCore := zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), l.level)
		cores = append(cores, consoleCore)
	}

	// Create logger
	core := zapcore.NewTee(cores...)
	l.zapLogger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	l.sugar = l.zapLogger.Sugar()

	return nil
}

// Close closes the logger and flushes any buffered data.
func (l *Logger) Close() error {
	if l.zapLogger != nil {
		l.zapLogger.Sync()
	}
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...zap.Field) {
	if l.zapLogger != nil {
		l.zapLogger.Debug(msg, fields...)
	}
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...zap.Field) {
	if l.zapLogger != nil {
		l.zapLogger.Info(msg, fields...)
	}
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...zap.Field) {
	if l.zapLogger != nil {
		l.zapLogger.Warn(msg, fields...)
	}
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...zap.Field) {
	if l.zapLogger != nil {
		l.zapLogger.Error(msg, fields...)
	}
}

// Debugf logs a formatted debug message.
func (l *Logger) Debugf(template string, args ...interface{}) {
	if l.sugar != nil {
		l.sugar.Debugf(template, args...)
	}
}

// Infof logs a formatted info message.
func (l *Logger) Infof(template string, args ...interface{}) {
	if l.sugar != nil {
		l.sugar.Infof(template, args...)
	}
}

// Warnf logs a formatted warning message.
func (l *Logger) Warnf(template string, args ...interface{}) {
	if l.sugar != nil {
		l.sugar.Warnf(template, args...)
	}
}

// Errorf logs a formatted error message.
func (l *Logger) Errorf(template string, args ...interface{}) {
	if l.sugar != nil {
		l.sugar.Errorf(template, args...)
	}
}

// WithFields returns a logger with additional fields.
func (l *Logger) WithFields(fields ...zap.Field) *zap.Logger {
	if l.zapLogger != nil {
		return l.zapLogger.With(fields...)
	}
	return nil
}

// LogTransfer logs a file transfer event.
func (l *Logger) LogTransfer(direction, protocol, localPath, remotePath string, size int64, duration time.Duration, err error) {
	fields := []zap.Field{
		zap.String("direction", direction),
		zap.String("protocol", protocol),
		zap.String("local_path", localPath),
		zap.String("remote_path", remotePath),
		zap.Int64("size_bytes", size),
		zap.Duration("duration", duration),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		l.Error("transfer failed", fields...)
	} else {
		speed := float64(size) / duration.Seconds() / 1024 / 1024 // MB/s
		fields = append(fields, zap.Float64("speed_mbps", speed))
		l.Info("transfer completed", fields...)
	}
}

// LogConnection logs a connection event.
func (l *Logger) LogConnection(protocol, host string, port int, connected bool, err error) {
	fields := []zap.Field{
		zap.String("protocol", protocol),
		zap.String("host", host),
		zap.Int("port", port),
		zap.Bool("connected", connected),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		l.Error("connection failed", fields...)
	} else if connected {
		l.Info("connected", fields...)
	} else {
		l.Info("disconnected", fields...)
	}
}

// Rotate rotates the log file if it exceeds max size.
func (l *Logger) Rotate(maxSize int64, maxBackups int) error {
	if l.logFile == nil || l.logPath == "" {
		return nil
	}

	info, err := l.logFile.Stat()
	if err != nil {
		return err
	}

	if info.Size() < maxSize {
		return nil
	}

	// Close current file
	l.logFile.Close()

	// Rotate backups
	for i := maxBackups - 1; i > 0; i-- {
		oldPath := l.logPath + "." + string(rune('0'+i))
		newPath := l.logPath + "." + string(rune('0'+i+1))
		os.Rename(oldPath, newPath)
	}

	// Rename current to .1
	os.Rename(l.logPath, l.logPath+".1")

	// Create new file
	l.logFile, err = os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	return err
}

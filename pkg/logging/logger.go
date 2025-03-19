package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// LogConfig holds configuration for the logger
type LogConfig struct {
	Directory     string `yaml:"directory"` // Directory for log files
	Prefix        string `yaml:"prefix"`    // Prefix for log files
	ConsoleOutput bool   `yaml:"console"`   // Whether to output to console
	FileOutput    bool   `yaml:"file"`      // Whether to output to file
	MaxSize       int64  `yaml:"max_size"`  // Maximum size of log file in MB
	MaxAge        int    `yaml:"max_age"`   // Maximum age of log files in days
	Compress      bool   `yaml:"compress"`  // Whether to compress old log files
}

// Logger represents a logger that writes to both file and console
type Logger struct {
	fileLogger    *log.Logger
	consoleLogger *log.Logger
	file          *os.File
	config        *LogConfig
}

// NewLoggerWithConfig creates a new logger with the given configuration
func NewLoggerWithConfig(config *LogConfig) (*Logger, error) {
	var fileLogger *log.Logger
	var consoleLogger *log.Logger
	var file *os.File

	// Create logs directory if file output is enabled
	if config.FileOutput {
		if err := os.MkdirAll(config.Directory, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %v", err)
		}

		// Create log file with timestamp
		timestamp := time.Now().Format("2006-01-02")
		logFile := filepath.Join(config.Directory, fmt.Sprintf("%s-%s.log", config.Prefix, timestamp))

		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %v", err)
		}
		file = f
		fileLogger = log.New(file, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	}

	// Create console logger if console output is enabled
	if config.ConsoleOutput {
		consoleLogger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	}

	return &Logger{
		fileLogger:    fileLogger,
		consoleLogger: consoleLogger,
		file:          file,
		config:        config,
	}, nil
}

// NewLogger creates a new logger with default configuration (for backward compatibility)
func NewLogger(logDir, prefix string) (*Logger, error) {
	config := &LogConfig{
		Directory:     logDir,
		Prefix:        prefix,
		ConsoleOutput: true,
		FileOutput:    true,
		MaxSize:       100, // 100MB
		MaxAge:        7,   // 7 days
		Compress:      true,
	}
	return NewLoggerWithConfig(config)
}

// Info logs an info message
func (l *Logger) Info(format string, v ...interface{}) {
	msg := fmt.Sprintf("[INFO] "+format, v...)
	if l.fileLogger != nil {
		l.fileLogger.Println(msg)
	}
	if l.consoleLogger != nil {
		l.consoleLogger.Println(msg)
	}
}

// Error logs an error message
func (l *Logger) Error(format string, v ...interface{}) {
	msg := fmt.Sprintf("[ERROR] "+format, v...)
	if l.fileLogger != nil {
		l.fileLogger.Println(msg)
	}
	if l.consoleLogger != nil {
		l.consoleLogger.Println(msg)
	}
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// RotateLogFile rotates the log file if it exceeds the configured max size
func (l *Logger) RotateLogFile() error {
	if !l.config.FileOutput || l.file == nil {
		return nil
	}

	info, err := l.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get log file info: %v", err)
	}

	if info.Size() > l.config.MaxSize*1024*1024 {
		l.file.Close()

		// Create new log file
		timestamp := time.Now().Format("2006-01-02-150405")
		oldPath := l.file.Name()
		newPath := filepath.Join(l.config.Directory,
			fmt.Sprintf("%s-%s.log", l.config.Prefix, timestamp))

		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("failed to rotate log file: %v", err)
		}

		// Open new log file
		file, err := os.OpenFile(oldPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open new log file: %v", err)
		}

		l.file = file
		l.fileLogger = log.New(file, "", log.Ldate|log.Ltime|log.Lmicroseconds)

		// Clean up old log files
		if err := l.cleanOldLogs(); err != nil {
			l.Error("Failed to clean old logs: %v", err)
		}
	}

	return nil
}

// cleanOldLogs removes log files older than MaxAge days
func (l *Logger) cleanOldLogs() error {
	if !l.config.FileOutput || l.config.MaxAge <= 0 {
		return nil
	}

	files, err := filepath.Glob(filepath.Join(l.config.Directory, fmt.Sprintf("%s-*.log*", l.config.Prefix)))
	if err != nil {
		return fmt.Errorf("failed to list log files: %v", err)
	}

	cutoff := time.Now().AddDate(0, 0, -l.config.MaxAge)

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(file); err != nil {
				l.Error("Failed to remove old log file %s: %v", file, err)
			}
		}
	}

	return nil
}

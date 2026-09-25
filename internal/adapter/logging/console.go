package logging

import (
	"fmt"
	"io"
	"os"
	"time"

	"ghostwire/internal/port"
)

type ConsoleLogger struct {
	level port.LogLevel
	sink  io.Writer
}

func NewConsoleLogger(level port.LogLevel) *ConsoleLogger {
	return &ConsoleLogger{level: level, sink: os.Stderr}
}

func NewConsoleLoggerTo(level port.LogLevel, sink io.Writer) *ConsoleLogger {
	return &ConsoleLogger{level: level, sink: sink}
}

func (l *ConsoleLogger) Level() port.LogLevel {
	return l.level
}

func (l *ConsoleLogger) Debug(message string, meta ...map[string]any) {
	l.log(port.LogDebug, message, meta)
}

func (l *ConsoleLogger) Info(message string, meta ...map[string]any) {
	l.log(port.LogInfo, message, meta)
}

func (l *ConsoleLogger) Warn(message string, meta ...map[string]any) {
	l.log(port.LogWarn, message, meta)
}

func (l *ConsoleLogger) Error(message string, meta ...map[string]any) {
	l.log(port.LogError, message, meta)
}

var levelTags = map[port.LogLevel]string{
	port.LogDebug: "DEBUG",
	port.LogInfo:  "INFO ",
	port.LogWarn:  "WARN ",
	port.LogError: "ERROR",
}

func (l *ConsoleLogger) log(level port.LogLevel, message string, meta []map[string]any) {
	if !port.IsLevelEnabled(l.level, level) {
		return
	}
	now := time.Now()
	line := fmt.Sprintf("%s %s %s", now.Format("15:04:05.000"), levelTags[level], message)
	if len(meta) > 0 && len(meta[0]) > 0 {
		line += fmt.Sprintf(" %v", meta[0])
	}
	fmt.Fprintln(l.sink, line)
}

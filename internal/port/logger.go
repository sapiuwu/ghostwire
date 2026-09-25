package port

type LogLevel string

const (
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
)

var levelOrder = map[LogLevel]int{
	LogDebug: 10,
	LogInfo:  20,
	LogWarn:  30,
	LogError: 40,
}

func IsLevelEnabled(configured, candidate LogLevel) bool {
	return levelOrder[candidate] >= levelOrder[configured]
}

type Logger interface {
	Level() LogLevel
	Debug(message string, meta ...map[string]any)
	Info(message string, meta ...map[string]any)
	Warn(message string, meta ...map[string]any)
	Error(message string, meta ...map[string]any)
}

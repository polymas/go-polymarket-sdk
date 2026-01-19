package internal

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 日志级别控制
// ============================================================================

// LogLevel 表示日志级别
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

var (
	// 当前日志级别，默认为 INFO（不输出 DEBUG）
	// 可以通过环境变量 LOG_LEVEL 设置：DEBUG, INFO, WARN, ERROR
	currentLogLevel LogLevel = LogLevelInfo
	logLevelOnce    sync.Once

	// customLogger 用于输出日志，不包含默认的时间戳和文件信息
	// 因为我们自己已经添加了这些信息
	customLogger     *log.Logger
	customLoggerOnce sync.Once
)

// getCustomLogger 获取自定义日志记录器，不包含默认的时间戳和文件信息
func getCustomLogger() *log.Logger {
	customLoggerOnce.Do(func() {
		// 创建一个新的logger，Flags设为0，不添加默认的时间戳和文件信息
		// 使用标准logger的输出目标（可能是文件+控制台）
		customLogger = log.New(log.Writer(), "", 0)
	})
	return customLogger
}

// initLogLevel 初始化日志级别
func initLogLevel() {
	logLevelOnce.Do(func() {
		levelStr := os.Getenv("LOG_LEVEL")
		switch strings.ToUpper(levelStr) {
		case "DEBUG":
			currentLogLevel = LogLevelDebug
		case "INFO", "":
			currentLogLevel = LogLevelInfo
		case "WARN":
			currentLogLevel = LogLevelWarn
		case "ERROR":
			currentLogLevel = LogLevelError
		default:
			currentLogLevel = LogLevelInfo
		}
	})
}

// shouldLog 判断是否应该输出指定级别的日志
func shouldLog(level LogLevel) bool {
	initLogLevel()
	return level >= currentLogLevel
}

// logWithCaller 输出日志，包含调用者的文件名和行号
// 格式: [级别] 时间 文件:行号 信息
func logWithCaller(level string, format string, v ...interface{}) {
	// 获取调用者的信息
	// Caller(0) = logWithCaller 自己
	// Caller(1) = LogError/LogInfo/LogWarn 等包装函数
	// Caller(2) = 实际调用 LogError/LogInfo/LogWarn 的代码位置
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "unknown"
		line = 0
	} else {
		// 只保留文件名，不包含完整路径
		if idx := strings.LastIndex(file, "/"); idx >= 0 {
			file = file[idx+1:]
		}
	}

	// 获取当前时间
	now := time.Now().Format("2006/01/02 15:04:05")

	// 构建格式: [级别] 时间 文件:行号 信息
	// 使用自定义logger，Flags为0，避免 log.Printf 添加额外的时间戳和文件位置
	callerFormat := fmt.Sprintf("[%s] %s %s:%d: %s", level, now, file, line, format)
	getCustomLogger().Printf(callerFormat, v...)
}

// LogInfo 输出 INFO 级别日志
func LogInfo(format string, v ...interface{}) {
	if shouldLog(LogLevelInfo) {
		logWithCaller("I", format, v...)
	}
}

// LogWarn 输出 WARN 级别日志
func LogWarn(format string, v ...interface{}) {
	if shouldLog(LogLevelWarn) {
		logWithCaller("W", format, v...)
	}
}

// LogError 输出 ERROR 级别日志
func LogError(format string, v ...interface{}) {
	if shouldLog(LogLevelError) {
		logWithCaller("E", format, v...)
	}
}

// LogFatal 输出 FATAL 级别日志并退出程序
func LogFatal(format string, v ...interface{}) {
	logWithCaller("F", format, v...)
	os.Exit(1)
}

// LogDebug 输出 DEBUG 级别日志
// 格式: [D] 时间 文件:行号 信息
func LogDebug(format string, v ...interface{}) {
	if shouldLog(LogLevelDebug) {
		// 获取调用者的信息（跳过当前函数）
		_, file, line, ok := runtime.Caller(1)
		if !ok {
			file = "unknown"
			line = 0
		} else {
			// 只保留文件名，不包含完整路径
			if idx := strings.LastIndex(file, "/"); idx >= 0 {
				file = file[idx+1:]
			}
		}

		// 获取当前时间
		now := time.Now().Format("2006/01/02 15:04:05")

		// 构建格式: [D] 时间 文件:行号 信息
		// 使用自定义logger，Flags为0，避免 log.Printf 添加额外的时间戳
		callerFormat := fmt.Sprintf("[D] %s %s:%d: %s", now, file, line, format)
		getCustomLogger().Printf(callerFormat, v...)
	}
}

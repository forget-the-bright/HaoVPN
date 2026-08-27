// Package logger provides leveled logging with optional file rotation.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Level represents log severity.
type Level int32

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var levelNames = map[Level]string{
	LevelTrace: "TRACE",
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
	LevelFatal: "FATAL",
}

// Config configures the global logger.
type Config struct {
	Level      string
	File       string
	MaxSizeMB  int
	MaxBackups int
}

// Logger is the application logger.
type Logger struct {
	level    atomic.Int32
	mu       sync.Mutex
	std      *log.Logger
	file     *log.Logger
	fileW    io.Closer
	livePath string // 同步落盘观测文件路径（*.live.log）
	prefix   string
}

var defaultLogger = New(Config{Level: "info"})

// Init configures the global logger.
func Init(cfg Config) error {
	l, err := NewWithError(cfg)
	if err != nil {
		return err
	}
	defaultLogger = l
	return nil
}

// New creates a logger with the given config.
func New(cfg Config) *Logger {
	l, _ := NewWithError(cfg)
	return l
}

// NewWithError creates a logger and returns setup errors.
func NewWithError(cfg Config) (*Logger, error) {
	l := &Logger{
		std: log.New(os.Stdout, "", log.LstdFlags),
	}
	l.SetLevel(ParseLevel(cfg.Level))

	if cfg.File != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.File), 0o755); err != nil {
			return nil, fmt.Errorf("create log dir: %w", err)
		}
		maxSize := cfg.MaxSizeMB
		if maxSize <= 0 {
			maxSize = 100
		}
		maxBackups := cfg.MaxBackups
		if maxBackups <= 0 {
			maxBackups = 7
		}
		lj := &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			Compress:   true,
		}
		// live 文件：每次 Init 覆盖，每行 Sync，便于 Get-Content -Wait / AI 读盘观测（不依赖 sudo 控制台）
		livePath := liveLogPath(cfg.File)
		live, err := os.OpenFile(livePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = lj.Close()
			return nil, fmt.Errorf("open live log: %w", err)
		}
		l.fileW = &multiCloser{a: lj, b: live}
		l.file = log.New(io.MultiWriter(lj, &syncFileWriter{f: live}), "", log.LstdFlags)
		l.livePath = livePath
	}
	return l, nil
}

// liveLogPath 在配置日志旁生成 *.live.log（如 server.log → server.live.log）。
func liveLogPath(file string) string {
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)
	if ext == "" {
		return file + ".live.log"
	}
	return base + ".live" + ext
}

// syncFileWriter 每次 Write 后 Sync，保证盘上立刻可见。
type syncFileWriter struct {
	f *os.File
}

func (s *syncFileWriter) Write(p []byte) (int, error) {
	n, err := s.f.Write(p)
	if err != nil {
		return n, err
	}
	return n, s.f.Sync()
}

// multiCloser 关闭滚动日志与 live 文件。
type multiCloser struct {
	a, b io.Closer
}

func (m *multiCloser) Close() error {
	err1 := m.a.Close()
	err2 := m.b.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// LivePath 返回当前 live 日志路径（无文件日志时为空）。
func (l *Logger) LivePath() string { return l.livePath }

// LivePath 返回全局 logger 的 live 日志路径。
func LivePath() string { return defaultLogger.LivePath() }

// ParseLevel converts a string level to Level.
func ParseLevel(s string) Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TRACE":
		return LevelTrace
	case "DEBUG":
		return LevelDebug
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	default:
		return LevelInfo
	}
}

// SetLevel sets the minimum log level.
func (l *Logger) SetLevel(lv Level) { l.level.Store(int32(lv)) }

// SetPrefix sets a logger prefix for all messages.
func (l *Logger) SetPrefix(prefix string) { l.prefix = prefix }

func (l *Logger) enabled(lv Level) bool {
	return int32(lv) >= l.level.Load()
}

func (l *Logger) log(lv Level, skip int, format string, args ...any) {
	if !l.enabled(lv) {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if lv >= LevelError {
		msg = msg + "\n" + stackTrace(skip+2)
	}
	name := levelNames[lv]
	if l.prefix != "" {
		name = l.prefix + " " + name
	}
	line := fmt.Sprintf("[%s] %s", name, msg)

	if lv >= LevelWarn {
		RecordRecent(line)
	}

	if fn := getHistoryWriter(); fn != nil && lv >= LevelInfo {
		fn(levelNames[lv], line)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.std.Println(line)
	if l.file != nil {
		l.file.Println(line)
	}
	if sink := getSink(); sink != nil {
		sink(lv, line)
	}
}

func stackTrace(skip int) string {
	const depth = 32
	pcs := make([]uintptr, depth)
	n := runtime.Callers(skip, pcs)
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])
	var b strings.Builder
	b.WriteString("stack trace:")
	for {
		frame, more := frames.Next()
		if !more {
			break
		}
		if strings.Contains(frame.File, "runtime/") {
			continue
		}
		fmt.Fprintf(&b, "\n  %s:%d %s", frame.File, frame.Line, frame.Function)
	}
	return b.String()
}

// Close closes file handles.
func (l *Logger) Close() error {
	if l.fileW != nil {
		return l.fileW.Close()
	}
	return nil
}

func (l *Logger) Trace(format string, args ...any) { l.log(LevelTrace, 2, format, args...) }
func (l *Logger) Debug(format string, args ...any) { l.log(LevelDebug, 2, format, args...) }
func (l *Logger) Info(format string, args ...any)  { l.log(LevelInfo, 2, format, args...) }
func (l *Logger) Warn(format string, args ...any)  { l.log(LevelWarn, 2, format, args...) }
func (l *Logger) Error(format string, args ...any) { l.log(LevelError, 2, format, args...) }
func (l *Logger) Fatal(format string, args ...any) {
	l.log(LevelFatal, 2, format, args...)
	os.Exit(1)
}

// Package-level helpers using the default logger.

func Trace(format string, args ...any) { defaultLogger.Trace(format, args...) }
func Debug(format string, args ...any) { defaultLogger.Debug(format, args...) }
func Info(format string, args ...any)  { defaultLogger.Info(format, args...) }
func Warn(format string, args ...any)  { defaultLogger.Warn(format, args...) }
func Error(format string, args ...any) { defaultLogger.Error(format, args...) }
func Fatal(format string, args ...any) { defaultLogger.Fatal(format, args...) }
func SetLevel(lv Level)                { defaultLogger.SetLevel(lv) }
func Close() error                     { return defaultLogger.Close() }

// Sink 额外日志回调（GUI 滚动区等）；勿在回调内再打 logger，以免递归。
type Sink func(level Level, line string)

var (
	sinkMu sync.Mutex
	sinkFn Sink
)

// SetSink 设置全局日志旁路；传 nil 清除。
func SetSink(fn Sink) {
	sinkMu.Lock()
	sinkFn = fn
	sinkMu.Unlock()
}

func getSink() Sink {
	sinkMu.Lock()
	defer sinkMu.Unlock()
	return sinkFn
}

var (
	historyMu sync.Mutex
	historyFn func(level, line string)
)

// SetHistoryWriter 注册结构化历史日志写入（logstore 异步入库）。
func SetHistoryWriter(fn func(level, line string)) {
	historyMu.Lock()
	historyFn = fn
	historyMu.Unlock()
}

func getHistoryWriter() func(level, line string) {
	historyMu.Lock()
	defer historyMu.Unlock()
	return historyFn
}

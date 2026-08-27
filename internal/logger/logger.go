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

	"haovpn/internal/fileutil"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Level 表示日志严重级别；数值越大越严重，过滤时仅输出 ≥ 当前阈值的级别。
type Level int32

const (
	// LevelTrace 最细粒度跟踪；默认配置下通常不输出。
	LevelTrace Level = iota
	// LevelDebug 开发调试信息。
	LevelDebug
	// LevelInfo 常规运行信息。
	LevelInfo
	// LevelWarn 可恢复异常或策略告警。
	LevelWarn
	// LevelError 错误；输出时附带 stack trace。
	LevelError
	// LevelFatal 致命错误；记录后 os.Exit(1)。
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

// Config 初始化全局或独立 Logger 时的文件与级别选项。
//
// 字段：
//   Level — 字符串阈值（trace/debug/info/warn/error/fatal）；空或未识别时视为 info。
//   File — 滚动日志路径；空则仅 stdout，不写 live 文件。
//   MaxSizeMB — 单文件上限 MB；≤0 时默认 100。
//   MaxBackups — 保留历史文件数；≤0 时默认 7。
type Config struct {
	Level      string
	File       string
	MaxSizeMB  int
	MaxBackups int
}

// Logger 应用结构化日志器：stdout + 可选 lumberjack 滚动文件与 live 同步文件。
//
// 字段：
//   level — atomic 当前最低输出级别。
//   mu — 保护 std/file 并发 Write。
//   std / file — 标准输出与文件 logger；file 为 nil 时仅控制台。
//   fileW — 关闭滚动与 live 文件的 Closer。
//   livePath — 旁路 *.live.log 路径，便于 tail 观测。
//   prefix — 每条日志级别名前的前缀（如组件名）。
//
// 线程安全：SetLevel 无锁 atomic；log 路径持 mu。
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

// Init 用 cfg 重建并替换包级 defaultLogger。
//
// 参数：cfg 同 New；File 非空时创建目录与 live 日志。
// 返回：打开文件或建目录失败时 err；成功则后续 Trace/Info 等走新实例。
// 副作用：替换全局 defaultLogger；旧实例句柄不由 Init 关闭。
func Init(cfg Config) error {
	l, err := NewWithError(cfg)
	if err != nil {
		return err
	}
	defaultLogger = l
	return nil
}

// New 按 cfg 创建 Logger；配置错误被忽略（见 NewWithError）。
//
// 参数：cfg 见 Config。
// 返回：始终非 nil *Logger；文件打开失败时仍返回仅 stdout 的实例（err 被丢弃）。
// 副作用：可能创建日志目录与文件。
func New(cfg Config) *Logger {
	l, _ := NewWithError(cfg)
	return l
}

// NewWithError 创建 Logger 并将 setup 错误返回给调用方。
//
// 参数：cfg 见 Config；File 非空时同时挂载 lumberjack 与 live MultiWriter。
// 返回：*Logger 与 err；目录/ live 文件失败时 err 非 nil 且 Logger 为 nil。
// 副作用：成功时可能 TRUNC 覆盖 live 日志文件。
func NewWithError(cfg Config) (*Logger, error) {
	l := &Logger{
		std: log.New(os.Stdout, "", log.LstdFlags),
	}
	l.SetLevel(ParseLevel(cfg.Level))

	if cfg.File != "" {
		if err := fileutil.EnsureParentDir(cfg.File, 0o755); err != nil {
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

// LivePath 返回本实例旁路 live 日志的绝对/配置路径。
//
// 参数：无。
// 返回：未配置文件日志时为空串。
// 副作用：无。
func (l *Logger) LivePath() string { return l.livePath }

// LivePath 返回包级 defaultLogger 的 live 日志路径。
//
// 参数：无。
// 返回：同 (*Logger).LivePath；Init 前或未配 File 时为空。
// 副作用：无。
func LivePath() string { return defaultLogger.LivePath() }

// ParseLevel 将配置字符串解析为 Level 枚举。
//
// 参数：s 大小写不敏感；支持 trace/debug/info/warn/warning/error/fatal。
// 返回：匹配级别；无法识别时默认 LevelInfo。
// 副作用：无。
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

// SetLevel 设置本实例最低输出级别（atomic 存储）。
//
// 参数：lv 须为 Level 常量之一。
// 返回：无。
// 副作用：立即影响后续 enabled 判断；可并发与 log 调用交错。
func (l *Logger) SetLevel(lv Level) { l.level.Store(int32(lv)) }

// SetPrefix 为每条日志的级别标签前添加固定前缀。
//
// 参数：prefix 可为空以清除；非空时显示为「prefix LEVEL」。
// 返回：无。
// 副作用：无锁写字段；宜在 Init 后、并发打日志前调用。
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

// Close 关闭滚动日志与 live 文件句柄。
//
// 参数：无。
// 返回：关闭 fileW 时的错误；无文件时 nil。
// 副作用：释放文件描述符；之后不应再向 file logger 写入。
func (l *Logger) Close() error {
	if l.fileW != nil {
		return l.fileW.Close()
	}
	return nil
}

// Trace 输出 TRACE 级格式化日志（低于阈值时不输出）。
func (l *Logger) Trace(format string, args ...any) { l.log(LevelTrace, 2, format, args...) }
// Debug 输出 DEBUG 级格式化日志。
func (l *Logger) Debug(format string, args ...any) { l.log(LevelDebug, 2, format, args...) }
// Info 输出 INFO 级格式化日志；若已注册 historyWriter 则异步入库。
func (l *Logger) Info(format string, args ...any)  { l.log(LevelInfo, 2, format, args...) }
// Warn 输出 WARN 级日志并写入近期日志环形缓冲。
func (l *Logger) Warn(format string, args ...any)  { l.log(LevelWarn, 2, format, args...) }
// Error 输出 ERROR 级日志并附加 stack trace。
func (l *Logger) Error(format string, args ...any) { l.log(LevelError, 2, format, args...) }
// Fatal 输出 FATAL 级日志后调用 os.Exit(1) 终止进程。
func (l *Logger) Fatal(format string, args ...any) {
	l.log(LevelFatal, 2, format, args...)
	os.Exit(1)
}

// 以下包级函数委托 defaultLogger，供业务代码直接 logger.Info 调用。

// Trace 使用包级 logger 输出 TRACE 日志。
func Trace(format string, args ...any) { defaultLogger.Trace(format, args...) }
// Debug 使用包级 logger 输出 DEBUG 日志。
func Debug(format string, args ...any) { defaultLogger.Debug(format, args...) }
// Info 使用包级 logger 输出 INFO 日志。
func Info(format string, args ...any)  { defaultLogger.Info(format, args...) }
// Warn 使用包级 logger 输出 WARN 日志。
func Warn(format string, args ...any)  { defaultLogger.Warn(format, args...) }
// Error 使用包级 logger 输出 ERROR 日志。
func Error(format string, args ...any) { defaultLogger.Error(format, args...) }
// Fatal 使用包级 logger 输出 FATAL 并退出进程。
func Fatal(format string, args ...any) { defaultLogger.Fatal(format, args...) }
// SetLevel 设置包级 logger 的最低输出级别。
func SetLevel(lv Level)                { defaultLogger.SetLevel(lv) }
// Close 关闭包级 logger 的文件句柄。
func Close() error                     { return defaultLogger.Close() }

// Sink 额外日志旁路回调（如 GUI 滚动区实时显示）。
//
// 参数约定：level 为已格式化的级别名；line 为完整一行 `[LEVEL] msg`。
// 注意：回调内勿再调用本包 logger，以免递归。
type Sink func(level Level, line string)

var (
	sinkMu sync.Mutex
	sinkFn Sink
)

// SetSink 注册或清除全局日志旁路回调。
//
// 参数：fn 非 nil 时每条 ≥ 配置阈值的日志在写 stdout/file 后调用；nil 清除。
// 返回：无。
// 副作用：持 sinkMu 替换全局 sinkFn。
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

// SetHistoryWriter 注册结构化历史日志写入器（通常对接 logstore.Enqueue）。
//
// 参数：fn 接收级别名与完整行；nil 清除；仅 INFO 及以上触发。
// 返回：无。
// 副作用：持 historyMu 替换全局 historyFn；写入应在 fn 内异步完成以免阻塞打日志。
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

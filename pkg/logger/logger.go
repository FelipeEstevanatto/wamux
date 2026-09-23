package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	applog "github.com/evolution-foundation/evolution-go/pkg/applog"
	"github.com/evolution-foundation/evolution-go/pkg/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// logQueueSize bounds how many formatted lines may wait for the writer.
	// Producers never block on a full queue: they fall back to a direct write,
	// so a line is never dropped because the disk is slow.
	logQueueSize = 8192

	// maxBatchBytes caps one buffered write. The writer goroutine keeps
	// appending queued lines until the queue is momentarily empty or this many
	// bytes accumulate, which turns a burst of lines into a handful of writes
	// instead of one syscall each.
	maxBatchBytes = 256 * 1024
)

type LoggerManager struct {
	config  *config.Config
	loggers map[string]*Logger
	mu      sync.RWMutex
}

// Logger writes one instance's log lines to its own lumberjack file.
//
// Formatting and JSON encoding happen on the caller's goroutine; the encoded
// line is then handed to a per-logger writer goroutine over a buffered channel.
// This keeps the old behaviour (every line is persisted) while removing the
// serialisation that used to make concurrent logging slower than sequential
// logging, and while batching disk writes under load.
type Logger struct {
	config     *config.Config
	instanceId string
	writer     *lumberjack.Logger

	// writeMu serialises access to writer. It is held only for the duration of a
	// write, never across formatting or JSON encoding.
	writeMu sync.Mutex

	// queue carries encoded lines to the writer goroutine.
	queue chan []byte

	// flush lets a caller wait until everything queued so far has been written,
	// so the log viewer never misses the most recent lines.
	flush chan chan struct{}

	done      chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool
	wg        sync.WaitGroup
}

type LogEntry struct {
	Timestamp  time.Time       `json:"timestamp"`
	Level      string          `json:"level"`
	InstanceId string          `json:"instance_id"`
	Message    string          `json:"message"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

func NewLoggerManager(config *config.Config) *LoggerManager {
	// Garante que o diretório base de logs existe
	if err := os.MkdirAll(config.LogDirectory, 0755); err != nil {
		applog.Logger.LogError("Falha ao criar diretório base de logs: %v", err)
	}

	return &LoggerManager{
		config:  config,
		loggers: make(map[string]*Logger),
	}
}

func (lm *LoggerManager) GetLogger(instanceId string) *Logger {
	lm.mu.RLock()
	logger, exists := lm.loggers[instanceId]
	lm.mu.RUnlock()

	if exists {
		return logger
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Verificar novamente após obter o lock de escrita
	if logger, exists = lm.loggers[instanceId]; exists {
		return logger
	}

	// Criar novo logger para a instância
	logger = newLogger(instanceId, lm.config)
	lm.loggers[instanceId] = logger
	return logger
}

// Flush drains the instance's pending log lines, if a logger exists for it. It
// is used by the log viewer so it reads a file that includes everything written
// up to the request, rather than racing the writer goroutine.
func (lm *LoggerManager) Flush(instanceId string) {
	lm.mu.RLock()
	logger := lm.loggers[instanceId]
	lm.mu.RUnlock()
	if logger != nil {
		logger.Flush()
	}
}

func newLogger(instanceId string, config *config.Config) *Logger {
	// Garante que o diretório existe
	logPath := filepath.Join(config.LogDirectory, instanceId)
	os.MkdirAll(logPath, 0755)

	logFile := filepath.Join(logPath, "instance.log")

	writer := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    config.LogMaxSize,
		MaxBackups: config.LogMaxBackups,
		MaxAge:     config.LogMaxAge,
		Compress:   config.LogCompress,
	}

	logger := &Logger{
		config:     config,
		instanceId: instanceId,
		writer:     writer,
		queue:      make(chan []byte, logQueueSize),
		flush:      make(chan chan struct{}),
		done:       make(chan struct{}),
	}

	logger.wg.Add(1)
	go logger.run()
	return logger
}

func (l *Logger) LogInfo(format string, args ...interface{}) {
	l.log("INFO", format, args...)
}

// LogError writes to the instance file and still surfaces on the process log
// (stdout/stderr), so failures stay visible in `docker logs`. Info/debug lines
// are file-only: forwarding every one of them to the process-wide logger was
// the duplicate sink that made the hot path expensive.
func (l *Logger) LogError(format string, args ...interface{}) {
	l.log("ERROR", format, args...)
	applog.Logger.LogError(format, args...)
}

func (l *Logger) LogWarn(format string, args ...interface{}) {
	l.log("WARN", format, args...)
	applog.Logger.LogWarn(format, args...)
}

func (l *Logger) LogDebug(format string, args ...interface{}) {
	l.log("DEBUG", format, args...)
}

// log formats, encodes and enqueues one line. The expensive parts (formatting
// and JSON encoding) run outside any lock; the queue send is non-blocking with
// a direct-write fallback, so logging never waits on a full buffer.
func (l *Logger) log(level string, format string, args ...interface{}) {
	entry := LogEntry{
		Timestamp:  time.Now(),
		Level:      level,
		InstanceId: l.instanceId,
		Message:    fmt.Sprintf(format, args...),
	}

	line, err := json.Marshal(entry)
	if err != nil {
		applog.Logger.LogError("Failed to marshal log entry: %v", err)
		return
	}
	line = append(line, '\n')

	if l.closed.Load() {
		l.writeDirect(line)
		return
	}

	select {
	case l.queue <- line:
	default:
		// The writer is behind: write this line inline rather than dropping it.
		l.writeDirect(line)
	}
}

func (l *Logger) writeDirect(line []byte) {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	if _, err := l.writer.Write(line); err != nil {
		applog.Logger.LogError("Failed to write log: %v", err)
	}
}

// run is the per-logger writer goroutine. It coalesces queued lines into a
// single Write when possible and drains fully on flush or close.
func (l *Logger) run() {
	defer l.wg.Done()

	buf := make([]byte, 0, maxBatchBytes)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		l.writeMu.Lock()
		_, err := l.writer.Write(buf)
		l.writeMu.Unlock()
		if err != nil {
			applog.Logger.LogError("Failed to write log: %v", err)
		}
		buf = buf[:0]
	}

	drain := func() {
		for {
			select {
			case line := <-l.queue:
				buf = append(buf, line...)
				if len(buf) >= maxBatchBytes {
					flush()
				}
			default:
				flush()
				return
			}
		}
	}

	for {
		select {
		case line := <-l.queue:
			buf = append(buf, line...)
			// Write now if the batch is full, or if nothing else is waiting —
			// the latter keeps a low-traffic logger near-real-time.
			if len(buf) >= maxBatchBytes || len(l.queue) == 0 {
				flush()
			}
		case ack := <-l.flush:
			drain()
			close(ack)
		case <-l.done:
			drain()
			return
		}
	}
}

// Flush blocks until every line queued before the call has been written.
func (l *Logger) Flush() {
	if l.closed.Load() {
		return
	}
	ack := make(chan struct{})
	select {
	case l.flush <- ack:
		<-ack
	case <-l.done:
	}
}

// Close flushes pending lines and closes the underlying writer. It is safe to
// call more than once.
func (l *Logger) Close() error {
	var err error
	l.closeOnce.Do(func() {
		l.closed.Store(true)
		close(l.done)
		l.wg.Wait()
		l.writeMu.Lock()
		err = l.writer.Close()
		l.writeMu.Unlock()
	})
	return err
}

// GetLogs retorna os logs da instância com filtros opcionais
func (l *Logger) GetLogs(startDate, endDate time.Time, level string, limit int) ([]LogEntry, error) {
	// Implementação movida para o service
	return nil, fmt.Errorf("método movido para instance_service")
}

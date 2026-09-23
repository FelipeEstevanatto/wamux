package logger

import (
	"io"
	"log"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
)

// benchLogger builds a per-instance logger backed by a throwaway directory and
// silences the process-wide applog sink (stdlib log) so the benchmark measures
// the per-instance file path without spewing to the test output.
func benchLogger(b *testing.B) *Logger {
	b.Helper()
	log.SetOutput(io.Discard)
	cfg := &config.Config{
		LogDirectory:  b.TempDir(),
		LogMaxSize:    100,
		LogMaxBackups: 1,
		LogMaxAge:     1,
	}
	return newLogger("bench-instance", cfg)
}

// BenchmarkLoggerSequential measures the cost of one log line on a single
// goroutine: format, marshal, and (before the async change) a synchronous write.
func BenchmarkLoggerSequential(b *testing.B) {
	l := benchLogger(b)
	defer l.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.LogInfo("message %d to %s", i, "5511999999999")
	}
}

// BenchmarkLoggerParallel measures contention: many goroutines logging through
// the same instance logger. Before the change they serialise on the logger
// mutex, which is held across the disk write.
func BenchmarkLoggerParallel(b *testing.B) {
	l := benchLogger(b)
	defer l.Close()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			l.LogInfo("message %d to %s", i, "5511999999999")
			i++
		}
	})
}

// BenchmarkLoggerParallelManyInstances models the real workload: several
// instances logging concurrently, each through its own logger. Before the
// change every line also hit the single process-wide applog (stdlib) logger,
// so the instances contend on that global lock too.
func BenchmarkLoggerParallelManyInstances(b *testing.B) {
	loggers := make([]*Logger, 8)
	for i := range loggers {
		loggers[i] = newLogger("bench-instance", &config.Config{
			LogDirectory:  b.TempDir(),
			LogMaxSize:    100,
			LogMaxBackups: 1,
			LogMaxAge:     1,
		})
	}
	defer func() {
		for _, l := range loggers {
			l.Close()
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			loggers[i%len(loggers)].LogInfo("message %d", i)
			i++
		}
	})
}

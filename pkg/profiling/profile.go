package profiling

import (
	"log/slog"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"time"
)

func EnableProfiling(stopTime time.Duration) {
	slog.Info("profiling enabled")

	err := os.Mkdir("prof", 0o755)
	if err != nil {
		slog.Error("failed to create profiling directory", "err", err)
	}

	cf, err := os.Create("./prof/cpu.prof")
	if err != nil {
		slog.Error("failed to start CPU profiling", "error", err)
	}
	pprof.StartCPUProfile(cf)

	mf, err := os.Create("./prof/memory.prof")
	if err != nil {
		slog.Error("failed to start memory profiling", "error", err)
	}
	pprof.WriteHeapProfile(mf)

	tc, err := os.Create("./prof/trace.prof")
	if err != nil {
		slog.Error("failed to start trace profiling", "error", err)
	}
	trace.Start(tc)

	stop := time.After(stopTime)

	go func() {
		<-stop
		pprof.StopCPUProfile()
		trace.Stop()
		cf.Close()
		mf.Close()
		tc.Close()
		slog.Info("finished the profiling")
	}()
}

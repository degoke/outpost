package top

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/goke/outpost/internal/inspect"
	"github.com/goke/outpost/internal/transport"
)

type Printer struct {
	Out io.Writer
}

func RunOnce(ctx context.Context, exec transport.Executor, out io.Writer) error {
	stats, err := inspect.ListContainerStats(ctx, exec)
	if err != nil {
		return err
	}
	if len(stats) == 0 {
		fmt.Fprintln(out, "No running containers")
		return nil
	}
	fmt.Fprintf(out, "%-12s %-20s %-8s %-12s %-12s %s\n", "CONTAINER", "PROJECT", "CPU%", "MEM", "MEM%", "LIMIT")
	for _, s := range stats {
		proj := s.Project
		if proj == "" {
			proj = "-"
		}
		name := s.Name
		if len(name) > 12 {
			name = name[:12]
		}
		fmt.Fprintf(out, "%-12s %-20s %7.1f%% %10s %10.1f%% %s\n",
			name, proj, s.CPUPercent, formatBytes(s.MemUsage), s.MemPercent, formatBytes(s.MemLimit))
	}
	return nil
}

func RunWatch(ctx context.Context, exec transport.Executor, out io.Writer, interval time.Duration) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if isTTY(out) {
			fmt.Fprint(out, "\033[H\033[2J")
		}
		if err := RunOnce(ctx, exec, out); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sigCh:
			return nil
		case <-ticker.C:
		}
	}
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

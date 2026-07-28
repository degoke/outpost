package transport

import (
	"fmt"
	"io"
)

type ProgressFunc func(written, total int64)

func CopyWithProgress(dst io.Writer, src io.Reader, total int64, label string, out io.Writer, onProgress ProgressFunc) (int64, error) {
	var reporter *progressReporter
	reader := src
	if out != nil {
		reporter = newProgressReporter(label, total, out)
		reader = io.TeeReader(src, reporter)
	}
	n, err := io.Copy(dst, reader)
	if reporter != nil {
		reporter.finish()
	}
	if onProgress != nil {
		onProgress(n, total)
	}
	return n, err
}

type progressReporter struct {
	label   string
	total   int64
	out     io.Writer
	written int64
}

func newProgressReporter(label string, total int64, out io.Writer) *progressReporter {
	return &progressReporter{label: label, total: total, out: out}
}

func (p *progressReporter) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	p.render()
	return len(b), nil
}

func (p *progressReporter) render() {
	if p.total > 0 {
		pct := float64(p.written) / float64(p.total) * 100
		fmt.Fprintf(p.out, "\r%s: %s / %s (%.0f%%)", p.label, formatBytes(p.written), formatBytes(p.total), pct)
		return
	}
	fmt.Fprintf(p.out, "\r%s: %s", p.label, formatBytes(p.written))
}

func (p *progressReporter) finish() {
	p.render()
	fmt.Fprintln(p.out)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGT"[exp])
}

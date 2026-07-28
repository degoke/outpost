package transport

import (
	"bytes"
	"strings"
	"testing"
)

func TestCopyWithProgress(t *testing.T) {
	var out bytes.Buffer
	src := strings.NewReader("hello world")
	dst := &bytes.Buffer{}
	_, err := CopyWithProgress(dst, src, int64(len("hello world")), "test", &out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dst.String() != "hello world" {
		t.Fatalf("got %q", dst.String())
	}
	if !strings.Contains(out.String(), "test:") {
		t.Fatalf("expected progress output, got %q", out.String())
	}
}

func TestFormatBytes(t *testing.T) {
	if formatBytes(512) != "512 B" {
		t.Fatalf("got %q", formatBytes(512))
	}
	if formatBytes(1536) != "1.5 KiB" {
		t.Fatalf("got %q", formatBytes(1536))
	}
}

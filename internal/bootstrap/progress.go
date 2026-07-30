package bootstrap

import (
	"context"

	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
)

// EnsureWithOut runs Ensure and reports progress when out is non-nil.
func EnsureWithOut(ctx context.Context, exec transport.Executor, out *output.Printer) error {
	if out != nil {
		out.Step("Checking remote dependencies...")
	}
	return Ensure(ctx, exec)
}

// EnsureIncusWithOut runs EnsureIncus and reports progress when out is non-nil.
func EnsureIncusWithOut(ctx context.Context, exec transport.Executor, out *output.Printer) error {
	if out != nil {
		out.Step("Ensuring Incus is available...")
	}
	return EnsureIncus(ctx, exec)
}

// EnsureKubernetesToolsWithOut runs EnsureKubernetesTools and reports progress when out is non-nil.
func EnsureKubernetesToolsWithOut(ctx context.Context, exec transport.Executor, out *output.Printer) error {
	if out != nil {
		out.Step("Ensuring Kubernetes tools are available...")
	}
	return EnsureKubernetesTools(ctx, exec)
}

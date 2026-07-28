package machine_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/machine"
	"github.com/degoke/outpost/internal/transport/mock"
)

func TestIncusCLIFallsBackToSudo(t *testing.T) {
	exec := mock.New()
	exec.Responses["incus list >/dev/null 2>&1"] = mockResp(1, "")
	exec.Responses["sudo incus list >/dev/null 2>&1"] = mockResp(0, "")

	svc := &machine.Service{Exec: exec}
	cli, err := machine.ResolveIncusCLIForTest(svc, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cli != "sudo incus" {
		t.Fatalf("got %q", cli)
	}
}

func TestIncusCommandUsesResolvedCLI(t *testing.T) {
	exec := mock.New()
	exec.Responses["incus list >/dev/null 2>&1"] = mockResp(0, "")

	svc := &machine.Service{Exec: exec}
	cmd, err := machine.IncusCommandForTest(svc, context.Background(), "launch 'local:ubuntu:24.04' 'outpost-dev'")
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "incus launch 'local:ubuntu:24.04' 'outpost-dev'" {
		t.Fatalf("got %q", cmd)
	}
}

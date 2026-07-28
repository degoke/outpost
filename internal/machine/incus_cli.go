package machine

import (
	"context"
	"fmt"

	"github.com/degoke/outpost/internal/transport"
)

func (s *Service) incusCLI(ctx context.Context) (string, error) {
	if s.cachedIncusCLI != "" {
		return s.cachedIncusCLI, nil
	}
	for _, cli := range []string{"incus", "sudo incus"} {
		code, err := s.Exec.Run(ctx, cli+" list >/dev/null 2>&1", transport.RunOpts{})
		if err != nil {
			return "", err
		}
		if code == 0 {
			s.cachedIncusCLI = cli
			return cli, nil
		}
	}
	return "", fmt.Errorf("cannot access incus daemon — if Incus was just installed, run the command again (outpost uses sudo until your session picks up incus-admin group membership)")
}

func (s *Service) incusCommand(ctx context.Context, args string) (string, error) {
	cli, err := s.incusCLI(ctx)
	if err != nil {
		return "", err
	}
	return cli + " " + args, nil
}

// ResolveIncusCLIForTest exposes incus CLI resolution for tests.
func ResolveIncusCLIForTest(s *Service, ctx context.Context) (string, error) {
	return s.incusCLI(ctx)
}

// IncusCommandForTest exposes incus command formatting for tests.
func IncusCommandForTest(s *Service, ctx context.Context, args string) (string, error) {
	return s.incusCommand(ctx, args)
}

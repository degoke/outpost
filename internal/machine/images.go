package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/goke/outpost/internal/transport"
)

func incusImagePullSpec(image string) (remoteRef, localAlias string, ok bool) {
	image = strings.TrimSpace(image)
	if image == "" {
		return "", "", false
	}
	if strings.HasPrefix(image, "images:") {
		return image, "", true
	}
	if strings.Contains(image, "/") {
		return "images:" + image, image, true
	}
	distro, version, found := strings.Cut(image, ":")
	if !found || distro == "" || version == "" {
		return "", "", false
	}
	switch distro {
	case "ubuntu":
		return "images:ubuntu/" + version, image, true
	default:
		return "images:" + distro + "/" + version, image, true
	}
}

// IncusImagePullSpecForTest exposes image pull mapping for tests.
func IncusImagePullSpecForTest(image string) (remoteRef, localAlias string, ok bool) {
	return incusImagePullSpec(image)
}

// incusLocalImageRef qualifies a user image for local Incus operations.
// Colon syntax (ubuntu:24.04) is otherwise parsed as remote:image.
func incusLocalImageRef(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return image
	}
	if strings.HasPrefix(image, "images:") || strings.HasPrefix(image, "local:") {
		return image
	}
	if strings.Contains(image, ":") {
		return "local:" + image
	}
	return image
}

// IncusLocalImageRefForTest exposes local image reference formatting for tests.
func IncusLocalImageRefForTest(image string) string {
	return incusLocalImageRef(image)
}

func (s *Service) ensureImage(ctx context.Context, image string) error {
	localImage := incusLocalImageRef(image)
	infoCmd := fmt.Sprintf("incus image info %s >/dev/null 2>&1", shellQuote(localImage))
	code, err := s.Exec.Run(ctx, infoCmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code == 0 {
		return nil
	}

	remoteRef, localAlias, ok := incusImagePullSpec(image)
	if !ok {
		return fmt.Errorf("image %q not found on host — use an alias like ubuntu:24.04 or images:ubuntu/24.04", image)
	}

	if s.Out != nil {
		s.Out.Info("Pulling Incus image %s...", image)
	}
	copyParts := []string{
		"incus image copy",
		shellQuote(remoteRef),
		"local:",
	}
	if localAlias != "" {
		copyParts = append(copyParts, "--alias", shellQuote(localAlias))
	}
	code, err = s.Exec.Run(ctx, strings.Join(copyParts, " "), transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("failed to pull image %q from %s — verify the host can reach images.linuxcontainers.org", image, remoteRef)
	}
	return nil
}

package upload

import (
	"io"

	"github.com/degoke/outpost/internal/transport"
)

// UploadFile uploads a local file to a remote path, optionally reporting byte progress.
func UploadFile(exec transport.Executor, local, remote string, progress io.Writer) error {
	if progress != nil {
		if uploader, ok := exec.(interface {
			UploadWithProgress(local, remote string, out io.Writer) error
		}); ok {
			return uploader.UploadWithProgress(local, remote, progress)
		}
	}
	return exec.Upload(local, remote)
}

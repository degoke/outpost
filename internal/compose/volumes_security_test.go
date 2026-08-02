package compose

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalArchivePathRejectsTraversal(t *testing.T) {
	_, err := localArchivePath("demo", "../../config")
	require.Error(t, err)
	_, err = localArchivePath("demo", "volume/name")
	require.Error(t, err)
}

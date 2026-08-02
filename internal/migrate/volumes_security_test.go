package migrate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalArchiveFileRejectsTraversal(t *testing.T) {
	_, err := localArchiveFile("demo", "../../config")
	require.Error(t, err)
}

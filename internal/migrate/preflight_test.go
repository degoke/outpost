package migrate_test

import (
	"errors"
	"testing"

	"github.com/degoke/outpost/internal/migrate"
	"github.com/stretchr/testify/require"
)

func TestPartialFailureHintAddsArchivePath(t *testing.T) {
	result := &migrate.Result{DockerExported: true}
	err := migrate.PartialFailureHintForTest(result, "demo", errors.New("compose up failed"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "compose up failed")
	require.Contains(t, err.Error(), "project host was not changed")
}

func TestPartialFailureHintSkipsWhenNothingExported(t *testing.T) {
	result := &migrate.Result{}
	err := migrate.PartialFailureHintForTest(result, "demo", errors.New("sync failed"))
	require.EqualError(t, err, "sync failed")
}

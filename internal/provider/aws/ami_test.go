package aws_test

import (
	"testing"

	"github.com/goke/outpost/internal/provider/aws"
	"github.com/stretchr/testify/require"
)

func TestUbuntuAMINamePatternsIncludeGP3(t *testing.T) {
	patterns := aws.UbuntuAMINamePatternsForTest()
	require.Contains(t, patterns, "ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*")
	require.Contains(t, patterns, "ubuntu/images/hvm-ssd/ubuntu-noble-24.04-amd64-server-*")
}

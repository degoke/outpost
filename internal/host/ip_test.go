package host_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/degoke/outpost/internal/host"
	"github.com/stretchr/testify/require"
)

func TestDetectPublicIPCIDR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("203.0.113.42\n"))
	}))
	defer srv.Close()

	cidr, err := host.DetectPublicIPCIDRFromURL(srv.URL)
	require.NoError(t, err)
	require.Equal(t, "203.0.113.42/32", cidr)
}

func TestDetectPublicIPCIDREmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("  \n"))
	}))
	defer srv.Close()

	_, err := host.DetectPublicIPCIDRFromURL(srv.URL)
	require.Error(t, err)
}

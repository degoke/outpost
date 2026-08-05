package host

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDetectPublicIPCIDR(t *testing.T) {
	old := publicIPClient
	publicIPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader("203.0.113.42\n"))}, nil
	})}
	defer func() { publicIPClient = old }()

	cidr, err := DetectPublicIPCIDRFromURL("https://example.test")
	require.NoError(t, err)
	require.Equal(t, "203.0.113.42/32", cidr)
}

func TestDetectPublicIPCIDREmptyResponse(t *testing.T) {
	old := publicIPClient
	publicIPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader("  \n"))}, nil
	})}
	defer func() { publicIPClient = old }()

	_, err := DetectPublicIPCIDRFromURL("https://example.test")
	require.Error(t, err)
}

func TestDetectPublicIPCIDRRejectsNonPublicAddresses(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "::1", "fc00::1"} {
		t.Run(ip, func(t *testing.T) {
			old := publicIPClient
			publicIPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(ip))}, nil
			})}
			defer func() { publicIPClient = old }()

			_, err := DetectPublicIPCIDRFromURL("https://example.test")
			require.Error(t, err)
		})
	}
}

func TestCloudSSHHostnamePrefersPublicIP(t *testing.T) {
	require.Equal(t, "3.135.233.178", cloudSSHHostname("3.135.233.178", "ec2-3-135-233-178.us-east-2.compute.amazonaws.com"))
	require.Equal(t, "ec2-3-135-233-178.us-east-2.compute.amazonaws.com", cloudSSHHostname("", "ec2-3-135-233-178.us-east-2.compute.amazonaws.com"))
	require.Equal(t, "", cloudSSHHostname("", ""))
}

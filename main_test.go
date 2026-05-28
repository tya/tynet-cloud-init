package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func stubLookup(fqdn string) func(string) ([]string, error) {
	return func(string) ([]string, error) {
		return []string{fqdn + "."}, nil
	}
}

func newTestServer(t *testing.T, lookup func(string) ([]string, error)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(newHandler("testdata/cloud-init", lookup))
	t.Cleanup(server.Close)
	return server
}

func TestServeCloudInit(t *testing.T) {
	nodes := []struct {
		hostname string // FQDN returned by PTR; must match a testdata dir
	}{
		{"pi2.tynet.us"},
		{"pi3.tynet.us"},
		{"testnode.vm"},
	}

	tests := []struct {
		name     string
		path     string
		wantBody string
	}{
		{"meta-data instance-id", "/meta-data", "instance-id:"},
		{"meta-data hostname", "/meta-data", "local-hostname:"},
		{"user-data header", "/user-data", "#cloud-config"},
		{"user-data ssh key", "/user-data", "ssh-ed25519 "},
	}

	for _, node := range nodes {
		server := newTestServer(t, stubLookup(node.hostname))
		for _, tt := range tests {
			t.Run(node.hostname+" "+tt.name, func(t *testing.T) {
				assertGet(t, server.URL+tt.path, http.StatusOK, tt.wantBody)
			})
		}
	}
}

func TestReverseLookupFailure(t *testing.T) {
	server := newTestServer(t, func(string) ([]string, error) {
		return nil, errors.New("no PTR record")
	})
	assertGet(t, server.URL+"/meta-data", http.StatusNotFound, "")
}

func TestUnknownHostname(t *testing.T) {
	// PTR resolves but the directory doesn't exist.
	server := newTestServer(t, stubLookup("nosuchhost.example"))
	assertGet(t, server.URL+"/meta-data", http.StatusNotFound, "")
}

func TestNodeOverrideRoute(t *testing.T) {
	// Stub returns a name with no matching dir; /node/ must bypass the lookup.
	server := newTestServer(t, stubLookup("nosuchhost.example"))
	assertGet(t, server.URL+"/node/pi2.tynet.us/meta-data", http.StatusOK, "local-hostname:")
}

func TestHealthcheckOK(t *testing.T) {
	server := newTestServer(t, stubLookup("nosuchhost.example"))
	assertGet(t, server.URL+"/healthcheck", http.StatusOK, "ok")
}

func TestHealthcheckMissingDir(t *testing.T) {
	server := httptest.NewServer(newHandler("/nonexistent/cloud-init", stubLookup("nosuchhost.example")))
	defer server.Close()

	resp, err := http.Get(server.URL + "/healthcheck")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func assertGet(t *testing.T, url string, wantCode int, wantBody string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantCode {
		t.Errorf("GET %s: got status %d, want %d", url, resp.StatusCode, wantCode)
	}
	if wantBody != "" {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), wantBody) {
			t.Errorf("GET %s: body does not contain %q", url, wantBody)
		}
	}
}

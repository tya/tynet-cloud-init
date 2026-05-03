package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeCloudInit(t *testing.T) {
	server := httptest.NewServer(newHandler("testdata/cloud-init"))
	defer server.Close()

	tests := []struct {
		name     string
		path     string
		wantCode int
		wantBody string
	}{
		// meta-data: must have an instance-id and local-hostname
		{name: "pi2 meta-data instance-id", path: "/dc-a6-32-8d-f3-ca/meta-data", wantCode: http.StatusOK, wantBody: "instance-id:"},
		{name: "pi2 meta-data hostname", path: "/dc-a6-32-8d-f3-ca/meta-data", wantCode: http.StatusOK, wantBody: "local-hostname:"},
		{name: "pi3 meta-data instance-id", path: "/dc-a6-32-80-2a-cc/meta-data", wantCode: http.StatusOK, wantBody: "instance-id:"},
		{name: "pi3 meta-data hostname", path: "/dc-a6-32-80-2a-cc/meta-data", wantCode: http.StatusOK, wantBody: "local-hostname:"},
		{name: "testnode meta-data instance-id", path: "/52-55-55-60-97-49/meta-data", wantCode: http.StatusOK, wantBody: "instance-id:"},
		{name: "testnode meta-data hostname", path: "/52-55-55-60-97-49/meta-data", wantCode: http.StatusOK, wantBody: "local-hostname:"},

		// user-data: must be a cloud-config document with an SSH key
		{name: "pi2 user-data header", path: "/dc-a6-32-8d-f3-ca/user-data", wantCode: http.StatusOK, wantBody: "#cloud-config"},
		{name: "pi2 user-data ssh key", path: "/dc-a6-32-8d-f3-ca/user-data", wantCode: http.StatusOK, wantBody: "ssh-ed25519 "},
		{name: "pi3 user-data header", path: "/dc-a6-32-80-2a-cc/user-data", wantCode: http.StatusOK, wantBody: "#cloud-config"},
		{name: "pi3 user-data ssh key", path: "/dc-a6-32-80-2a-cc/user-data", wantCode: http.StatusOK, wantBody: "ssh-ed25519 "},
		{name: "testnode user-data header", path: "/52-55-55-60-97-49/user-data", wantCode: http.StatusOK, wantBody: "#cloud-config"},
		{name: "testnode user-data ssh key", path: "/52-55-55-60-97-49/user-data", wantCode: http.StatusOK, wantBody: "ssh-ed25519 "},

		// unknown MAC → 404
		{name: "unknown mac", path: "/00-00-00-00-00-00/meta-data", wantCode: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tt.path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantCode {
				t.Errorf("got status %d, want %d", resp.StatusCode, tt.wantCode)
			}

			if tt.wantBody != "" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(body), tt.wantBody) {
					t.Errorf("body does not contain %q", tt.wantBody)
				}
			}
		})
	}
}

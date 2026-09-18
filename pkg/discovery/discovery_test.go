package discovery

import (
	"testing"
)

func TestBuildDialTarget(t *testing.T) {
	tests := []struct {
		serviceName string
		expected    string
	}{
		{
			serviceName: "user_service",
			expected:    "etcd:///user_service",
		},
		{
			serviceName: "message_service",
			expected:    "etcd:///message_service",
		},
		{
			serviceName: "group_service",
			expected:    "etcd:///group_service",
		},
	}

	for _, tt := range tests {
		target := BuildDialTarget(tt.serviceName)
		if target != tt.expected {
			t.Errorf("BuildDialTarget(%q) = %q, expected %q", tt.serviceName, target, tt.expected)
		}
	}
}

func TestServerInfo(t *testing.T) {
	info := ServerInfo{
		Name:   "test_service",
		Addr:   "127.0.0.1:8080",
		Weight: 10,
	}

	reg := &Register{
		info:        info,
		endpointKey: info.Name + "/" + info.Addr,
	}

	if reg.BuildRegPath(info) != "/test_service/127.0.0.1:8080" {
		t.Errorf("unexpected BuildRegPath: %s", reg.BuildRegPath(info))
	}

	if reg.endpointKey != "test_service/127.0.0.1:8080" {
		t.Errorf("unexpected endpointKey: %s", reg.endpointKey)
	}
}

package api

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNetworkRollbackContextSurvivesCanceledRequest(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	if requestCtx.Err() == nil {
		t.Fatal("request context should be canceled")
	}

	rollbackCtx, cancelRollback := newNetworkRollbackContext()
	defer cancelRollback()
	if rollbackCtx.Err() != nil {
		t.Fatalf("rollback context inherited request cancellation: %v", rollbackCtx.Err())
	}
}

func TestBuildNetworkCreateOptionsIgnoresDeprecatedIPv4Aliases(t *testing.T) {
	var req networkRequest
	if err := json.Unmarshal([]byte(`{
		"name": "legacy-network",
		"subnet": "172.30.0.0/16",
		"gateway": "172.30.0.1",
		"ipRange": "172.30.1.0/24"
	}`), &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}

	options := buildNetworkCreateOptions(req, "")
	if options.IPAM == nil {
		t.Fatal("expected IPAM options")
	}
	if len(options.IPAM.Config) != 0 {
		t.Fatalf("deprecated aliases unexpectedly affected IPv4 config: %#v", options.IPAM.Config)
	}
}

func TestBuildNetworkCreateOptionsUsesCurrentIPv4Fields(t *testing.T) {
	req := networkRequest{
		IPv4Subnet:  "172.31.0.0/16",
		IPv4Gateway: "172.31.0.1",
		IPv4IPRange: "172.31.1.0/24",
	}

	options := buildNetworkCreateOptions(req, "")
	if options.IPAM == nil || len(options.IPAM.Config) != 1 {
		t.Fatalf("expected one IPv4 config, got %#v", options.IPAM)
	}
	config := options.IPAM.Config[0]
	if config.Subnet != req.IPv4Subnet || config.Gateway != req.IPv4Gateway || config.IPRange != req.IPv4IPRange {
		t.Fatalf("unexpected IPv4 config: %#v", config)
	}
}

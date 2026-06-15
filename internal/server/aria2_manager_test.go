package server

import "testing"

func TestApplyManagedOptionPatchSyncsRPCPort(t *testing.T) {
	cfg := &Config{
		Aria2: Aria2Config{
			Managed:        true,
			ManagedRPCPort: defaultManagedRPCPort,
			RPCURL:         managedRPCURL(defaultManagedRPCPort),
			Options:        map[string]string{},
		},
	}

	patch, err := applyManagedOptionPatch(cfg, map[string]string{
		"rpc-listen-port":    "16888",
		"max-download-limit": "1M",
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if cfg.Aria2.ManagedRPCPort != 16888 {
		t.Fatalf("expected managed rpc port 16888, got %d", cfg.Aria2.ManagedRPCPort)
	}
	if cfg.Aria2.RPCURL != "http://127.0.0.1:16888/jsonrpc" {
		t.Fatalf("unexpected rpc url %q", cfg.Aria2.RPCURL)
	}
	if _, ok := patch["rpc-listen-port"]; ok {
		t.Fatal("expected rpc-listen-port to be handled by manager instead of aria2.changeGlobalOption")
	}
	if patch["max-download-limit"] != "1M" {
		t.Fatalf("expected max-download-limit to remain in patch, got %q", patch["max-download-limit"])
	}
}

func TestApplyManagedOptionPatchRejectsInvalidRPCPort(t *testing.T) {
	cfg := &Config{
		Aria2: Aria2Config{
			Managed:        true,
			ManagedRPCPort: defaultManagedRPCPort,
			RPCURL:         managedRPCURL(defaultManagedRPCPort),
		},
	}

	if _, err := applyManagedOptionPatch(cfg, map[string]string{"rpc-listen-port": "abc"}); err == nil {
		t.Fatal("expected invalid port error")
	}
}

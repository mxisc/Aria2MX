package server

import "testing"

func TestValidateManagedAria2PatchAllowsBTTrackerWhenSubscriptionDisabled(t *testing.T) {
	if err := validateManagedAria2Patch(map[string]string{"bt-tracker": "udp://a/announce"}, false); err != nil {
		t.Fatalf("expected patch to be allowed, got %v", err)
	}
}

func TestValidateManagedAria2PatchRejectsBTTrackerWhenSubscriptionEnabled(t *testing.T) {
	err := validateManagedAria2Patch(map[string]string{"bt-tracker": "udp://a/announce"}, true)
	if err == nil {
		t.Fatal("expected bt-tracker patch to be rejected when tracker subscription is enabled")
	}
}

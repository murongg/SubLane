package audit

import "testing"

func TestContentSettingsAuditTarget(t *testing.T) {
	if !ValidTarget("settings.update", "settings", "content") {
		t.Fatal("content settings cannot be audited")
	}
}

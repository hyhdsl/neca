package web

import "testing"

func TestHighRiskSettingsIsWriteEndpoint(t *testing.T) {
	if !isWriteEndpoint("/settings/high-risk") {
		t.Fatal("/settings/high-risk must pass readOnlyGuard to reach handler checks")
	}
}

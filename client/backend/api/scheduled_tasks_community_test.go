//go:build community

package api

import (
	"testing"
)

func TestCommunityBuildRejectsProtectionScheduledTaskType(t *testing.T) {
	if validTaskType("protection_backup") {
		t.Fatal("community build must not accept application protection scheduled tasks")
	}
}

package database

import "testing"

func TestComposeProjectIdentityPersistsUnicodePathAndSource(t *testing.T) {
	setupNotificationTestDB(t)

	record := ComposeProjectIdentity{
		EnvironmentID:      LocalEnvironmentID,
		RelativePath:       "好好的11",
		ComposeProjectName: "compose-2cd3107f",
		Source:             "generated",
	}
	if err := UpsertComposeProjectIdentity(record); err != nil {
		t.Fatal(err)
	}

	got, found, err := GetComposeProjectIdentity(LocalEnvironmentID, "好好的11")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("identity was not persisted")
	}
	if got.ComposeProjectName != "compose-2cd3107f" || got.Source != "generated" {
		t.Fatalf("identity = %#v", got)
	}
}

func TestComposeProjectIdentityRejectsDuplicateProjectNameInEnvironment(t *testing.T) {
	setupNotificationTestDB(t)
	if err := UpsertComposeProjectIdentity(ComposeProjectIdentity{
		EnvironmentID: LocalEnvironmentID, RelativePath: "first", ComposeProjectName: "shared", Source: "directory",
	}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertComposeProjectIdentity(ComposeProjectIdentity{
		EnvironmentID: LocalEnvironmentID, RelativePath: "second", ComposeProjectName: "shared", Source: "docker_label",
	}); err == nil {
		t.Fatal("duplicate Compose project name should be rejected within one environment")
	}
}

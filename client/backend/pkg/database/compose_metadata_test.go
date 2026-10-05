package database

import "testing"

func TestComposeProjectRemarkUpsertAndClear(t *testing.T) {
	setupNotificationTestDB(t)

	if err := SetComposeProjectRemark(LocalEnvironmentID, "downloads", "下载器"); err != nil {
		t.Fatal(err)
	}
	if err := SetComposeProjectRemark(LocalEnvironmentID, "downloads", "下载服务"); err != nil {
		t.Fatal(err)
	}
	remark, err := GetComposeProjectRemark(LocalEnvironmentID, "downloads")
	if err != nil {
		t.Fatal(err)
	}
	if remark != "下载服务" {
		t.Fatalf("remark=%q", remark)
	}

	if err := SetComposeProjectRemark(LocalEnvironmentID, "downloads", "  "); err != nil {
		t.Fatal(err)
	}
	remark, err = GetComposeProjectRemark(LocalEnvironmentID, "downloads")
	if err != nil {
		t.Fatal(err)
	}
	if remark != "" {
		t.Fatalf("cleared remark=%q", remark)
	}
}

func TestDeleteComposeProjectMetadataOnlyRemovesMatchingProject(t *testing.T) {
	setupNotificationTestDB(t)
	for project, remark := range map[string]string{"one": "项目一", "two": "项目二"} {
		if err := SetComposeProjectRemark(LocalEnvironmentID, project, remark); err != nil {
			t.Fatal(err)
		}
	}

	if err := DeleteComposeProjectMetadata(LocalEnvironmentID, "one"); err != nil {
		t.Fatal(err)
	}
	remarks, err := ListComposeProjectRemarks(LocalEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := remarks["one"]; exists || remarks["two"] != "项目二" {
		t.Fatalf("unexpected remarks after delete: %#v", remarks)
	}
}

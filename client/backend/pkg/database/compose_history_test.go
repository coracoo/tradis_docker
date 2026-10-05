package database

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestSaveComposeHistoryDeduplicatesAndRetainsTenPerProject(t *testing.T) {
	setupComposeHistoryTestDB(t)

	for i := 0; i < 11; i++ {
		record := ComposeHistoryRecord{
			ID:             fmt.Sprintf("history-%02d", i),
			EnvironmentID:  "local",
			ProjectName:    "demo",
			ComposePath:    "docker-compose.yml",
			YAMLHash:       fmt.Sprintf("hash-%02d", i),
			SnapshotSealed: fmt.Sprintf("sealed-%02d", i),
			SummaryJSON:    `{"total":1}`,
			Source:         "manual_edit",
			CreatedAt:      fmt.Sprintf("2026-08-03 00:00:%02d", i),
		}
		inserted, err := SaveComposeHistory(record, 10)
		if err != nil {
			t.Fatalf("SaveComposeHistory(%d) error = %v", i, err)
		}
		if !inserted {
			t.Fatalf("SaveComposeHistory(%d) did not insert", i)
		}
	}

	items, err := ListComposeHistory("local", "demo", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 10 {
		t.Fatalf("retained history count = %d, want 10", len(items))
	}
	if items[0].ID != "history-10" || items[len(items)-1].ID != "history-01" {
		t.Fatalf("unexpected retention order: first=%q last=%q", items[0].ID, items[len(items)-1].ID)
	}

	inserted, err := SaveComposeHistory(ComposeHistoryRecord{
		ID:             "duplicate",
		EnvironmentID:  "local",
		ProjectName:    "demo",
		ComposePath:    "compose.yml",
		YAMLHash:       "hash-10",
		SnapshotSealed: "different-envelope",
		SummaryJSON:    `{}`,
		Source:         "manual_edit",
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("duplicate YAML hash created another history row")
	}
}

func TestComposeHistoryIsScopedAndDeletesOnlyMatchingProject(t *testing.T) {
	setupComposeHistoryTestDB(t)
	record := func(id, environment, project string) ComposeHistoryRecord {
		return ComposeHistoryRecord{
			ID:             id,
			EnvironmentID:  environment,
			ProjectName:    project,
			ComposePath:    "compose.yaml",
			YAMLHash:       "hash-" + id,
			SnapshotSealed: "sealed-" + id,
			SummaryJSON:    `{"total":2}`,
			Source:         "history_restore",
			CreatedAt:      "2026-08-03 01:00:00",
		}
	}
	for _, item := range []ComposeHistoryRecord{
		record("local-demo", "local", "demo"),
		record("remote-demo", "remote-a", "demo"),
		record("local-other", "local", "other"),
	} {
		if inserted, err := SaveComposeHistory(item, 10); err != nil || !inserted {
			t.Fatalf("SaveComposeHistory(%s) = %v, %v", item.ID, inserted, err)
		}
	}

	got, err := GetComposeHistory("local", "demo", "local-demo")
	if err != nil || got.SnapshotSealed != "sealed-local-demo" {
		t.Fatalf("GetComposeHistory() = %#v, %v", got, err)
	}
	if _, err := GetComposeHistory("remote-a", "demo", "local-demo"); err == nil {
		t.Fatal("GetComposeHistory crossed environment boundary")
	}

	if err := DeleteComposeHistoryForProject("local", "demo"); err != nil {
		t.Fatal(err)
	}
	if items, err := ListComposeHistory("local", "demo", 10); err != nil || len(items) != 0 {
		t.Fatalf("deleted project history remains: %#v, %v", items, err)
	}
	if items, err := ListComposeHistory("remote-a", "demo", 10); err != nil || len(items) != 1 {
		t.Fatalf("remote project history was deleted: %#v, %v", items, err)
	}
	if items, err := ListComposeHistory("local", "other", 10); err != nil || len(items) != 1 {
		t.Fatalf("other project history was deleted: %#v, %v", items, err)
	}
}

func TestDeleteComposeHistoryRemovesSingleScopedRecord(t *testing.T) {
	setupComposeHistoryTestDB(t)
	record := ComposeHistoryRecord{
		ID: "history-1", EnvironmentID: "local", ProjectName: "demo",
		ComposePath: "compose.yml", YAMLHash: "hash-1", SnapshotSealed: "sealed",
		SummaryJSON: `{}`, Source: "manual_edit",
	}
	if inserted, err := SaveComposeHistory(record, 10); err != nil || !inserted {
		t.Fatalf("SaveComposeHistory() = %v, %v", inserted, err)
	}
	if err := DeleteComposeHistory("remote", "demo", "history-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetComposeHistory("local", "demo", "history-1"); err != nil {
		t.Fatalf("wrong-scope delete removed record: %v", err)
	}
	if err := DeleteComposeHistory("local", "demo", "history-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetComposeHistory("local", "demo", "history-1"); err == nil {
		t.Fatal("record remains after scoped delete")
	}
}

func TestPruneComposeHistoryCanBeDeferredUntilConfigWriteSucceeds(t *testing.T) {
	setupComposeHistoryTestDB(t)
	for i := 0; i < 11; i++ {
		inserted, err := SaveComposeHistory(ComposeHistoryRecord{
			ID: fmt.Sprintf("deferred-%02d", i), EnvironmentID: "local", ProjectName: "demo",
			ComposePath: "compose.yml", YAMLHash: fmt.Sprintf("deferred-hash-%02d", i),
			SnapshotSealed: "sealed", SummaryJSON: `{}`, Source: "manual_edit",
			CreatedAt: fmt.Sprintf("2026-08-03 02:00:%02d", i),
		}, 11)
		if err != nil || !inserted {
			t.Fatalf("SaveComposeHistory(%d) = %v, %v", i, inserted, err)
		}
	}
	if err := PruneComposeHistory("local", "demo", 10); err != nil {
		t.Fatal(err)
	}
	items, err := ListComposeHistory("local", "demo", 20)
	if err != nil || len(items) != 10 || items[len(items)-1].ID != "deferred-01" {
		t.Fatalf("deferred prune result = %#v, %v", items, err)
	}
}

func TestImportComposeHistoryMergesIdempotentlyAndPrunesOnce(t *testing.T) {
	setupComposeHistoryTestDB(t)
	for i := 0; i < 9; i++ {
		inserted, err := SaveComposeHistory(composeHistoryFixture(
			fmt.Sprintf("existing-%02d", i),
			fmt.Sprintf("hash-%02d", i),
			fmt.Sprintf("2026-08-03 03:00:%02d", i),
		), 20)
		if err != nil || !inserted {
			t.Fatalf("SaveComposeHistory(%d) = %v, %v", i, inserted, err)
		}
	}

	records := []ComposeHistoryRecord{
		composeHistoryFixture("import-new-10", "hash-10", "2026-08-03 03:00:10"),
		composeHistoryFixture("import-new-11", "hash-11", "2026-08-03 03:00:11"),
		composeHistoryFixture("duplicate-hash", "hash-08", "2026-08-03 03:00:12"),
	}
	inserted, err := ImportComposeHistory("local", "demo", records, 10)
	if err != nil {
		t.Fatalf("ImportComposeHistory() error = %v", err)
	}
	if inserted != 2 {
		t.Fatalf("ImportComposeHistory() inserted = %d, want 2", inserted)
	}
	inserted, err = ImportComposeHistory("local", "demo", records, 10)
	if err != nil || inserted != 0 {
		t.Fatalf("idempotent ImportComposeHistory() = %d, %v", inserted, err)
	}

	items, err := ListComposeHistory("local", "demo", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 10 || items[0].ID != "import-new-11" || items[len(items)-1].ID != "existing-01" {
		t.Fatalf("unexpected imported history: %#v", items)
	}
}

func TestImportComposeHistoryIDConflictRollsBackWholeBatch(t *testing.T) {
	setupComposeHistoryTestDB(t)
	existing := composeHistoryFixture("history-conflict", "hash-existing", "2026-08-03 04:00:00")
	if inserted, err := SaveComposeHistory(existing, 10); err != nil || !inserted {
		t.Fatalf("SaveComposeHistory() = %v, %v", inserted, err)
	}

	conflict := composeHistoryFixture("history-conflict", "hash-conflict", "2026-08-03 04:00:02")
	inserted, err := ImportComposeHistory("local", "demo", []ComposeHistoryRecord{
		composeHistoryFixture("history-new", "hash-new", "2026-08-03 04:00:01"),
		conflict,
	}, 10)
	if err == nil || inserted != 0 {
		t.Fatalf("conflicting ImportComposeHistory() = %d, %v", inserted, err)
	}
	if _, err := GetComposeHistory("local", "demo", "history-new"); err == nil {
		t.Fatal("record inserted before the conflict was not rolled back")
	}
	got, err := GetComposeHistory("local", "demo", existing.ID)
	if err != nil || got.YAMLHash != existing.YAMLHash {
		t.Fatalf("existing record changed after conflict: %#v, %v", got, err)
	}
}

func composeHistoryFixture(id, yamlHash, createdAt string) ComposeHistoryRecord {
	return ComposeHistoryRecord{
		ID: id, EnvironmentID: "local", ProjectName: "demo", ComposePath: "compose.yml",
		YAMLHash: yamlHash, SnapshotSealed: "sealed-" + id, SummaryJSON: `{"total":1}`,
		Source: "manual_edit", CreatedAt: createdAt,
	}
}

func setupComposeHistoryTestDB(t *testing.T) {
	t.Helper()
	if err := Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := InitDB(filepath.Join(t.TempDir(), "compose-history.db")); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() { _ = Close() })
}

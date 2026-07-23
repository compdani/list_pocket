package main

import "testing"

func TestSQLiteAdvanceBatchCursorUsesCreatedAndRecordID(t *testing.T) {
	cursor := sqliteBatchCursor{}
	rows := []sqliteStoreSubscriberRow{
		{
			RecordID:  "sub-001",
			CreatedAt: "2026-03-26 14:00:00.000Z",
			UpdatedAt: "2026-03-26 14:01:00.000Z",
		},
		{
			RecordID:  "sub-002",
			CreatedAt: "2026-03-26 14:00:00.000Z",
			UpdatedAt: "2026-03-26 14:10:00.000Z",
		},
		{
			RecordID:  "sub-003",
			CreatedAt: "2026-03-26 14:00:01.000Z",
			UpdatedAt: "2026-03-26 14:30:00.000Z",
		},
	}

	cursor = sqliteAdvanceBatchCursor(cursor, rows, 2)
	if cursor.LastCreated != "2026-03-26 14:00:00.000Z" {
		t.Fatalf("expected last_created to track stable created_at, got %q", cursor.LastCreated)
	}
	if cursor.LastID != "sub-002" {
		t.Fatalf("expected last_id to track stable subscriber record id, got %q", cursor.LastID)
	}
}

func TestSQLiteAdvanceBatchCursorUnaffectedByUpdatedAtOnlyChanges(t *testing.T) {
	base := sqliteBatchCursor{}

	rowsBefore := []sqliteStoreSubscriberRow{
		{
			RecordID:  "sub-002",
			CreatedAt: "2026-03-26 14:00:00.000Z",
			UpdatedAt: "2026-03-26 14:10:00.000Z",
		},
	}
	rowsAfter := []sqliteStoreSubscriberRow{
		{
			RecordID:  "sub-002",
			CreatedAt: "2026-03-26 14:00:00.000Z",
			UpdatedAt: "2026-03-26 16:45:00.000Z", // eg: bounce/status side effect
		},
	}

	before := sqliteAdvanceBatchCursor(base, rowsBefore, 1)
	after := sqliteAdvanceBatchCursor(base, rowsAfter, 1)

	if before.LastCreated != after.LastCreated {
		t.Fatalf("cursor last_created changed due to updated_at-only change: before=%q after=%q", before.LastCreated, after.LastCreated)
	}
	if before.LastID != after.LastID {
		t.Fatalf("cursor last_id changed due to updated_at-only change: before=%q after=%q", before.LastID, after.LastID)
	}
}

func TestSQLiteAdvanceBatchCursorStaysMonotonicWhenLastSubscriberDropsOut(t *testing.T) {
	// Batch 1 processes sub-001 and sub-002; pointer is set to sub-002.
	cursor := sqliteAdvanceBatchCursor(sqliteBatchCursor{}, []sqliteStoreSubscriberRow{
		{
			RecordID:  "sub-001",
			CreatedAt: "2026-03-26 14:00:00.000Z",
		},
		{
			RecordID:  "sub-002",
			CreatedAt: "2026-03-26 14:00:00.000Z",
		},
	}, 2)

	// Between batches, sub-002 becomes ineligible (eg: unsubscribed/blocklisted),
	// so it is no longer present in fetched rows. The cursor must still move forward.
	next := sqliteAdvanceBatchCursor(cursor, []sqliteStoreSubscriberRow{
		{
			RecordID:  "sub-003",
			CreatedAt: "2026-03-26 14:00:01.000Z",
		},
	}, 1)

	if next.LastCreated != "2026-03-26 14:00:01.000Z" {
		t.Fatalf("expected cursor to advance to the next eligible row, got last_created=%q", next.LastCreated)
	}
	if next.LastID != "sub-003" {
		t.Fatalf("expected cursor to advance to sub-003, got last_id=%q", next.LastID)
	}
}

package core

import (
	"context"
	"testing"

	"github.com/compdani/list_pocket/models"
)

func boardsSetup(t *testing.T) (*Core, Board) {
	t.Helper()
	c := crmSetup(t)
	board, err := c.CreateBoard("Sales Deals", []string{"New", "Won"})
	if err != nil {
		t.Fatal(err)
	}
	return c, board
}

func boardMove(t *testing.T, c *Core, board, sub, stage string, position *int) BoardMove {
	t.Helper()
	out, err := c.MoveBoardCard(BoardCardMove{Board: board, Subscriber: sub, Stage: stage, Position: position, Actor: "Sam"})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func boardStageCards(t *testing.T, c *Core, board, stage string) []string {
	t.Helper()
	got, err := c.GetBoard(board)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range got.Stages {
		if s.Key == stage {
			out := []string{}
			for _, card := range s.Cards {
				out = append(out, card.Subscriber)
			}
			return out
		}
	}
	t.Fatalf("no stage %q on %+v", stage, got)
	return nil
}

func sameCards(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBoards_pipelineIsSeededAsAnOrdinaryBoard(t *testing.T) {
	c := crmSetup(t)
	board, err := c.GetBoard("pipeline")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"lead", "prospect", "customer", "inactive"}
	if len(board.Stages) != len(want) {
		t.Fatalf("got %+v", board.Stages)
	}
	for i, key := range want {
		if board.Stages[i].Key != key {
			t.Fatalf("stage %d: got %q, want %q", i, board.Stages[i].Key, key)
		}
	}
	if err := c.DeleteBoard("pipeline"); err != nil {
		t.Fatal(err)
	}
}

func TestBoards_keysAreUniqueAndSurviveRenames(t *testing.T) {
	c, board := boardsSetup(t)
	if board.Key != "sales_deals" || board.Stages[1].Key != "won" {
		t.Fatalf("got %+v", board)
	}
	second, err := c.CreateBoard("Sales  deals!", []string{"Same", "same"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Key != "sales_deals_2" || second.Stages[1].Key != "same_2" {
		t.Fatalf("got %+v", second)
	}
	other, err := c.CreateBoard("日本語", nil)
	if err != nil {
		t.Fatal(err)
	}
	if other.Key != "board" || len(other.Stages) != len(boardDefaultStages) {
		t.Fatalf("got %+v", other)
	}
	renamed, err := c.RenameBoard(board.Key, "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Key != board.Key || renamed.Name != "Renamed" {
		t.Fatalf("got %+v", renamed)
	}
	if _, err := c.CreateBoard("   ", nil); err == nil {
		t.Fatal("expected an empty name to be refused")
	}
}

func TestBoardCards_oneStagePerBoardAndManyBoards(t *testing.T) {
	c, board := boardsSetup(t)
	sub := crmTestContact(t, c, "a@example.com").RecordID

	add := boardMove(t, c, board.Key, sub, "new", nil)
	if !add.Changed || add.PreviousStage != "" || add.Stage != "new" {
		t.Fatalf("add: %+v", add)
	}
	move := boardMove(t, c, board.Key, sub, "won", nil)
	if !move.Changed || move.PreviousStage != "new" || move.PreviousStageName != "New" {
		t.Fatalf("move: %+v", move)
	}
	if again := boardMove(t, c, board.Key, sub, "won", nil); again.Changed {
		t.Fatalf("a move to the same stage should change nothing: %+v", again)
	}
	boardMove(t, c, "pipeline", sub, "lead", nil)

	boards, err := c.GetContactBoards(sub)
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) != 2 {
		t.Fatalf("got %+v", boards)
	}
	if got := boardStageCards(t, c, board.Key, "new"); len(got) != 0 {
		t.Fatalf("still in the old stage: %v", got)
	}

	if _, err := c.MoveBoardCard(BoardCardMove{Board: board.Key, Subscriber: sub, Stage: "nope"}); err == nil {
		t.Fatal("expected an unknown stage to be refused")
	}
	if _, err := c.MoveBoardCard(BoardCardMove{Board: "nope", Subscriber: sub, Stage: "new"}); err == nil {
		t.Fatal("expected an unknown board to be refused")
	}

	removed, err := c.RemoveBoardCard(board.Key, sub, "Sam")
	if err != nil || !removed.Changed || removed.PreviousStage != "won" || removed.Stage != "" {
		t.Fatalf("remove: %+v, %v", removed, err)
	}
	if removed, err = c.RemoveBoardCard(board.Key, sub, "Sam"); err != nil || removed.Changed {
		t.Fatalf("removing twice: %+v, %v", removed, err)
	}
}

func TestBoardCards_ordering(t *testing.T) {
	c, board := boardsSetup(t)
	a := crmTestContact(t, c, "a@example.com").RecordID
	b := crmTestContact(t, c, "b@example.com").RecordID
	d := crmTestContact(t, c, "d@example.com").RecordID
	at := func(n int) *int { return &n }

	// A new card goes on top.
	boardMove(t, c, board.Key, a, "new", nil)
	boardMove(t, c, board.Key, b, "new", nil)
	boardMove(t, c, board.Key, d, "new", nil)
	if got := boardStageCards(t, c, board.Key, "new"); !sameCards(got, []string{d, b, a}) {
		t.Fatalf("got %v", got)
	}

	// Position doesn't count the card itself.
	if m := boardMove(t, c, board.Key, d, "new", at(2)); m.Changed {
		t.Fatalf("a reorder isn't a stage change: %+v", m)
	}
	if got := boardStageCards(t, c, board.Key, "new"); !sameCards(got, []string{b, a, d}) {
		t.Fatalf("to the end: got %v", got)
	}
	boardMove(t, c, board.Key, a, "new", at(0))
	if got := boardStageCards(t, c, board.Key, "new"); !sameCards(got, []string{a, b, d}) {
		t.Fatalf("to the top: got %v", got)
	}
	boardMove(t, c, board.Key, d, "new", at(1))
	if got := boardStageCards(t, c, board.Key, "new"); !sameCards(got, []string{a, d, b}) {
		t.Fatalf("to the middle: got %v", got)
	}
	boardMove(t, c, board.Key, b, "new", at(99))
	if got := boardStageCards(t, c, board.Key, "new"); !sameCards(got, []string{a, d, b}) {
		t.Fatalf("past the end: got %v", got)
	}

	// A move keeps its place when no position is given.
	boardMove(t, c, board.Key, d, "new", nil)
	if got := boardStageCards(t, c, board.Key, "new"); !sameCards(got, []string{a, d, b}) {
		t.Fatalf("no position: got %v", got)
	}

	// Into another stage at a position.
	boardMove(t, c, board.Key, a, "won", nil)
	boardMove(t, c, board.Key, b, "won", at(1))
	if got := boardStageCards(t, c, board.Key, "won"); !sameCards(got, []string{a, b}) {
		t.Fatalf("across stages: got %v", got)
	}
	if _, err := c.MoveBoardCard(BoardCardMove{Board: board.Key, Subscriber: a, Stage: "won", Position: at(-1)}); err == nil {
		t.Fatal("expected a negative position to be refused")
	}
}

func TestBoardStages_reorderAndEdit(t *testing.T) {
	c, board := boardsSetup(t)
	got, err := c.ReorderBoardStages(board.Key, []string{"won", "new"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Stages[0].Key != "won" {
		t.Fatalf("got %+v", got.Stages)
	}
	for _, bad := range [][]string{{"won"}, {"won", "won"}, {"won", "nope"}} {
		if _, err := c.ReorderBoardStages(board.Key, bad); err == nil {
			t.Fatalf("expected %v to be refused", bad)
		}
	}
	if got, err = c.AddBoardStage(board.Key, "Lost", "error"); err != nil {
		t.Fatal(err)
	}
	if last := got.Stages[len(got.Stages)-1]; last.Key != "lost" || last.Color != "error" {
		t.Fatalf("got %+v", last)
	}
	bad := "chartreuse"
	if _, err := c.UpdateBoardStage(board.Key, "lost", BoardStageInput{Color: &bad}); err == nil {
		t.Fatal("expected an unknown color to be refused")
	}
	name := "Closed"
	if got, err = c.UpdateBoardStage(board.Key, "lost", BoardStageInput{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if last := got.Stages[len(got.Stages)-1]; last.Key != "lost" || last.Name != "Closed" {
		t.Fatalf("a rename should keep the key: %+v", last)
	}
}

func TestBoardStages_deleteMovesCards(t *testing.T) {
	c, board := boardsSetup(t)
	a := crmTestContact(t, c, "a@example.com").RecordID
	b := crmTestContact(t, c, "b@example.com").RecordID
	boardMove(t, c, board.Key, a, "won", nil)
	boardMove(t, c, board.Key, b, "new", nil)

	if _, err := c.DeleteBoardStage(board.Key, "new", "", "Sam"); err == nil {
		t.Fatal("expected deleting a stage with cards and no target to be refused")
	}
	if _, err := c.DeleteBoardStage(board.Key, "new", "new", "Sam"); err == nil {
		t.Fatal("expected moving cards to the same stage to be refused")
	}
	if _, err := c.DeleteBoardStage(board.Key, "new", "won", "Sam"); err != nil {
		t.Fatal(err)
	}
	if got := boardStageCards(t, c, board.Key, "won"); !sameCards(got, []string{a, b}) {
		t.Fatalf("moved cards should go to the end: %v", got)
	}
	if _, err := c.DeleteBoardStage(board.Key, "won", "", "Sam"); err == nil {
		t.Fatal("expected deleting the last stage to be refused")
	}

	timeline := boardTimeline(t, c, b, nil)
	if len(timeline.Events) != 2 || timeline.Events[0].Status != "moved" {
		t.Fatalf("a stage delete should show on the timeline: %+v", timeline.Events)
	}
}

func boardTimeline(t *testing.T, c *Core, subRecID string, types []string) *models.UnifiedContactTimeline {
	t.Helper()
	ids, err := c.ResolveSubscriberIDs(nil, []string{subRecID})
	if err != nil || len(ids) != 1 {
		t.Fatalf("resolving subscriber: %v, %v", ids, err)
	}
	out, err := c.GetUnifiedContactTimeline(context.Background(), TimelineQueryParams{SubscriberID: ids[0], EventTypes: types})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBoardMoves_showOnTheTimeline(t *testing.T) {
	c, board := boardsSetup(t)
	sub := crmTestContact(t, c, "a@example.com").RecordID
	at := func(n int) *int { return &n }

	boardMove(t, c, board.Key, sub, "new", nil)
	boardMove(t, c, board.Key, sub, "new", at(0))
	boardMove(t, c, board.Key, sub, "won", nil)
	if _, err := c.RemoveBoardCard(board.Key, sub, "Workflow"); err != nil {
		t.Fatal(err)
	}

	// Filtering on only this type must not return an empty page.
	got := boardTimeline(t, c, sub, []string{models.TimelineEventBoardMove})
	if got.Total != 3 || len(got.Events) != 3 {
		t.Fatalf("a reorder shouldn't be logged, got %+v", got)
	}
	statuses := []string{got.Events[0].Status, got.Events[1].Status, got.Events[2].Status}
	if !sameCards(statuses, []string{"removed", "moved", "added"}) {
		t.Fatalf("got %v", statuses)
	}
	if got.Events[0].Actor.Label != "Workflow" || got.Events[1].Actor.Label != "Sam" {
		t.Fatalf("got %+v", got.Events)
	}

	// History outlives the board, and reads with the old names.
	if err := c.DeleteBoard(board.Key); err != nil {
		t.Fatal(err)
	}
	after := boardTimeline(t, c, sub, nil)
	if after.Total != 3 {
		t.Fatalf("got %+v", after)
	}
	if want := `{"board":"Sales Deals","from_stage":"New","to_stage":"Won"}`; string(after.Events[1].Metadata) != want {
		t.Fatalf("got %s", after.Events[1].Metadata)
	}
}

func TestBoardData_removedWithSubscriber(t *testing.T) {
	c, board := boardsSetup(t)
	sub := crmTestContact(t, c, "a@example.com").RecordID
	boardMove(t, c, board.Key, sub, "new", nil)
	if _, err := c.db.Exec(`DELETE FROM subscribers WHERE id = ?`, sub); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"board_cards", "board_card_events"} {
		var n int
		if err := c.db.Get(&n, `SELECT COUNT(*) FROM `+table+` WHERE subscriber = ?`, sub); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s kept %d rows", table, n)
		}
	}
}

func TestBoardsMigration_movesStagesAndGrantsAccess(t *testing.T) {
	c := crmSetup(t)
	app := c.db.PocketBase()
	sub := crmTestContact(t, c, "a@example.com").RecordID
	older := crmTestContact(t, c, "b@example.com").RecordID

	if _, err := c.db.Exec(`INSERT INTO contact_profiles (subscriber, company) VALUES (?, 'Acme')`, sub); err != nil {
		t.Fatal(err)
	}
	for i, r := range [][2]string{{"plain", `["subscribers:get_all"]`}, {"manager", `["subscribers:manage","subscribers:get_all"]`}, {"none", `["campaigns:get"]`}} {
		if _, err := c.db.Exec(`INSERT INTO roles (name, type, permissions, legacy_id) VALUES (?, 'user', ?, ?)`, r[0], r[1], 1000+i); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := app.DB().NewQuery(boardsTestRollback).Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.Exec(`
INSERT INTO contact_profiles (subscriber, stage, updated) VALUES (?, 'customer', '2026-01-01 10:00:00.000Z')
ON CONFLICT (subscriber) DO UPDATE SET stage = excluded.stage, updated = excluded.updated`, sub); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.Exec(`INSERT INTO contact_profiles (subscriber, stage, updated) VALUES (?, 'customer', '2025-01-01 10:00:00.000Z')`, older); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DB().NewQuery(`DELETE FROM _migrations WHERE file = '000054_boards.go'`).Execute(); err != nil {
		t.Fatal(err)
	}
	if err := app.RunAppMigrations(); err != nil {
		t.Fatal(err)
	}

	if got := boardStageCards(t, c, "pipeline", "customer"); !sameCards(got, []string{sub, older}) {
		t.Fatalf("most recently moved should come first: %v", got)
	}
	profile, err := c.GetContactProfile(sub)
	if err != nil || profile.Company != "Acme" {
		t.Fatalf("got %+v, %v", profile, err)
	}
	has := func(role, perm string) bool {
		var n int
		if err := c.db.Get(&n, `SELECT COUNT(*) FROM roles r, json_each(r.permissions) WHERE r.name = ? AND value = ?`, role, perm); err != nil {
			t.Fatal(err)
		}
		return n > 0
	}
	if !has("plain", "boards:get") || has("plain", "boards:manage") {
		t.Fatal("a reader should only be able to read boards")
	}
	if !has("manager", "boards:get") || !has("manager", "boards:manage") {
		t.Fatal("a manager should be able to manage boards")
	}
	if has("none", "boards:get") {
		t.Fatal("a role without subscriber access shouldn't get boards")
	}
}

// boardsTestRollback undoes the boards migration so a test can run it on known data. The
// real down migration is exercised against a copy of a real database.
const boardsTestRollback = `
ALTER TABLE contact_profiles ADD COLUMN stage TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_contact_profiles_stage ON contact_profiles (stage, updated);
DROP TABLE IF EXISTS board_card_events;
DROP TABLE IF EXISTS board_cards;
DROP TABLE IF EXISTS board_stages;
DROP TABLE IF EXISTS boards;`

package core

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/jmoiron/sqlx"
)

const (
	maxBoardCardsPerStage = 200
	maxBoardStages        = 50
	maxBoardNameChars     = 200
	maxBoardKeyChars      = 40
	boardStageDefault     = "default"
)

var (
	boardDefaultStages = []string{"To do", "In progress", "Done"}
	boardStageColors   = map[string]bool{boardStageDefault: true, "info": true, "success": true, "warning": true, "error": true, "primary": true, "secondary": true}
	boardKeyStrip      = regexp.MustCompile(`[^a-z0-9]+`)
)

// BoardCard is one contact on a board.
type BoardCard struct {
	Subscriber string `db:"subscriber" json:"subscriber"`
	Name       string `db:"name" json:"name"`
	Email      string `db:"email" json:"email"`
	Company    string `db:"company" json:"company"`
	OwnerName  string `db:"owner_name" json:"owner_name"`
	OpenTasks  int    `db:"open_tasks" json:"open_tasks"`
	MovedAt    string `db:"moved_at" json:"moved_at"`
}

// BoardStage is one column of a board. Total can exceed the cards returned, because each
// stage is capped.
type BoardStage struct {
	ID       int64       `db:"id" json:"-"`
	Board    int64       `db:"board" json:"-"`
	Key      string      `db:"key" json:"key"`
	Name     string      `db:"name" json:"name"`
	Color    string      `db:"color" json:"color"`
	Position int         `db:"position" json:"position"`
	Total    int         `db:"total" json:"total"`
	Cards    []BoardCard `db:"-" json:"cards"`
}

// Board is a kanban board. Key never changes, so workflows can refer to it through renames.
type Board struct {
	ID     int64        `db:"id" json:"-"`
	Key    string       `db:"key" json:"key"`
	Name   string       `db:"name" json:"name"`
	Stages []BoardStage `db:"-" json:"stages"`
}

// ContactBoard says where a contact is on one board.
type ContactBoard struct {
	Board     string `db:"board" json:"board"`
	BoardName string `db:"board_name" json:"board_name"`
	Stage     string `db:"stage" json:"stage"`
	StageName string `db:"stage_name" json:"stage_name"`
	Color     string `db:"color" json:"color"`
}

// BoardMove describes a change to a contact's place on a board. Changed is false if the
// contact stayed in the same stage. Stage is empty after a removal, and PreviousStage is
// empty after an add.
type BoardMove struct {
	Subscriber        string `json:"subscriber"`
	Board             string `json:"board"`
	BoardName         string `json:"board_name"`
	Stage             string `json:"stage"`
	StageName         string `json:"stage_name"`
	PreviousStage     string `json:"previous_stage"`
	PreviousStageName string `json:"previous_stage_name"`
	Changed           bool   `json:"changed"`
}

// BoardCardMove says where to put a card. A nil Position keeps a card's place if it stays
// in its stage, and otherwise puts it at the top. Position counts cards in the stage from
// zero, not including the card itself.
type BoardCardMove struct {
	Board      string
	Subscriber string
	Stage      string
	Position   *int
	Actor      string
}

// BoardStageInput changes the fields that aren't nil.
type BoardStageInput struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func boardKey(name, fallback string) string {
	key := strings.Trim(boardKeyStrip.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if len(key) > maxBoardKeyChars {
		key = strings.TrimRight(key[:maxBoardKeyChars], "_")
	}
	if key == "" {
		return fallback
	}
	return key
}

// uniqueBoardKey adds a number to a key if taken says it's in use.
func uniqueBoardKey(key string, taken func(string) (bool, error)) (string, error) {
	candidate := key
	for i := 2; ; i++ {
		used, err := taken(candidate)
		if err != nil || !used {
			return candidate, err
		}
		candidate = fmt.Sprintf("%s_%d", key, i)
	}
}

func cleanBoardName(name, what string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", apperr.BadRequest(what + " name is required")
	}
	if len([]rune(name)) > maxBoardNameChars {
		return "", apperr.BadRequest(what + " name is too long")
	}
	return name, nil
}

func cleanBoardColor(color string) (string, error) {
	color = strings.TrimSpace(color)
	if color == "" {
		return boardStageDefault, nil
	}
	if !boardStageColors[color] {
		return "", apperr.BadRequest("invalid stage color")
	}
	return color, nil
}

func (c *Core) boardByKey(q sqlx.Queryer, key string) (Board, error) {
	var out Board
	err := sqlx.Get(q, &out, `SELECT id, key, name FROM boards WHERE key = ?`, strings.TrimSpace(key))
	if errors.Is(err, sql.ErrNoRows) {
		return out, apperr.NotFound(fmt.Sprintf("board %q not found", key))
	}
	return out, err
}

func (c *Core) boardStageByKey(q sqlx.Queryer, board Board, key string) (BoardStage, error) {
	var out BoardStage
	err := sqlx.Get(q, &out, `SELECT id, board, key, name, color, position, 0 AS total FROM board_stages WHERE board = ? AND key = ?`, board.ID, strings.TrimSpace(key))
	if errors.Is(err, sql.ErrNoRows) {
		return out, apperr.NotFound(fmt.Sprintf("stage %q not found on board %q", key, board.Key))
	}
	return out, err
}

const boardStagesQuery = `
SELECT st.id, st.board, st.key, st.name, st.color, st.position,
  (SELECT COUNT(*) FROM board_cards c JOIN subscribers s ON s.id = c.subscriber WHERE c.stage = st.id) AS total
FROM board_stages st WHERE st.board = ? ORDER BY st.position, st.id`

func (c *Core) boardStages(boardID int64) ([]BoardStage, error) {
	out := []BoardStage{}
	if err := c.db.Select(&out, boardStagesQuery, boardID); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Cards = []BoardCard{}
	}
	return out, nil
}

// ListBoards returns every board with its stages and how many cards each holds, without
// the cards.
func (c *Core) ListBoards() ([]Board, error) {
	out := []Board{}
	if err := c.db.Select(&out, `SELECT id, key, name FROM boards ORDER BY position, id`); err != nil {
		return nil, err
	}
	for i := range out {
		stages, err := c.boardStages(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Stages = stages
	}
	return out, nil
}

// GetBoard returns a board with the first cards of each stage.
func (c *Core) GetBoard(key string) (Board, error) {
	out, err := c.boardByKey(c.db, key)
	if err != nil {
		return out, err
	}
	if out.Stages, err = c.boardStages(out.ID); err != nil {
		return out, err
	}
	for i := range out.Stages {
		if err := c.db.Select(&out.Stages[i].Cards, `
SELECT c.subscriber, COALESCE(s.name, '') AS name, s.email, COALESCE(p.company, '') AS company,
  `+crmUserNameOf("u")+` AS owner_name,
  (SELECT COUNT(*) FROM contact_tasks t WHERE t.subscriber = c.subscriber AND t.status = 'open') AS open_tasks,
  c.moved_at
FROM board_cards c
JOIN subscribers s ON s.id = c.subscriber
LEFT JOIN contact_profiles p ON p.subscriber = c.subscriber
LEFT JOIN users u ON u.id = p.owner
WHERE c.stage = ?
ORDER BY c.position, c.moved_at DESC LIMIT ?`, out.Stages[i].ID, maxBoardCardsPerStage); err != nil {
			return out, err
		}
	}
	return out, nil
}

// CreateBoard adds a board. With no stage names, it starts with a simple set.
func (c *Core) CreateBoard(name string, stageNames []string) (Board, error) {
	name, err := cleanBoardName(name, "board")
	if err != nil {
		return Board{}, err
	}
	if len(stageNames) == 0 {
		stageNames = boardDefaultStages
	}
	if len(stageNames) > maxBoardStages {
		return Board{}, apperr.BadRequest("a board has too many stages")
	}
	tx, err := c.db.Beginx()
	if err != nil {
		return Board{}, err
	}
	defer tx.Rollback()

	key, err := uniqueBoardKey(boardKey(name, "board"), func(k string) (bool, error) {
		var n int
		err := tx.Get(&n, `SELECT COUNT(*) FROM boards WHERE key = ?`, k)
		return n > 0, err
	})
	if err != nil {
		return Board{}, err
	}
	res, err := tx.Exec(`INSERT INTO boards (key, name, position) VALUES (?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM boards))`, key, name)
	if err != nil {
		return Board{}, err
	}
	boardID, err := res.LastInsertId()
	if err != nil {
		return Board{}, err
	}
	used := map[string]bool{}
	for i, stageName := range stageNames {
		stageName, err := cleanBoardName(stageName, "stage")
		if err != nil {
			return Board{}, err
		}
		stageKey, _ := uniqueBoardKey(boardKey(stageName, "stage"), func(k string) (bool, error) { return used[k], nil })
		used[stageKey] = true
		if _, err := tx.Exec(`INSERT INTO board_stages (board, key, name, position) VALUES (?, ?, ?, ?)`, boardID, stageKey, stageName, i); err != nil {
			return Board{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Board{}, err
	}
	return c.GetBoard(key)
}

// RenameBoard changes a board's name. Its key stays the same.
func (c *Core) RenameBoard(key, name string) (Board, error) {
	name, err := cleanBoardName(name, "board")
	if err != nil {
		return Board{}, err
	}
	board, err := c.boardByKey(c.db, key)
	if err != nil {
		return board, err
	}
	if _, err := c.db.Exec(`UPDATE boards SET name = ?, updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now') WHERE id = ?`, name, board.ID); err != nil {
		return board, err
	}
	return c.GetBoard(key)
}

// DeleteBoard removes a board and its cards. The contacts' move history stays.
func (c *Core) DeleteBoard(key string) error {
	board, err := c.boardByKey(c.db, key)
	if err != nil {
		return err
	}
	tx, err := c.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM board_cards WHERE board = ?`, `DELETE FROM board_stages WHERE board = ?`, `DELETE FROM boards WHERE id = ?`} {
		if _, err := tx.Exec(q, board.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddBoardStage adds a stage at the end of a board.
func (c *Core) AddBoardStage(boardRef, name, color string) (Board, error) {
	name, err := cleanBoardName(name, "stage")
	if err != nil {
		return Board{}, err
	}
	if color, err = cleanBoardColor(color); err != nil {
		return Board{}, err
	}
	board, err := c.boardByKey(c.db, boardRef)
	if err != nil {
		return board, err
	}
	var count int
	if err := c.db.Get(&count, `SELECT COUNT(*) FROM board_stages WHERE board = ?`, board.ID); err != nil {
		return board, err
	}
	if count >= maxBoardStages {
		return board, apperr.BadRequest("a board has too many stages")
	}
	key, err := uniqueBoardKey(boardKey(name, "stage"), func(k string) (bool, error) {
		var n int
		err := c.db.Get(&n, `SELECT COUNT(*) FROM board_stages WHERE board = ? AND key = ?`, board.ID, k)
		return n > 0, err
	})
	if err != nil {
		return board, err
	}
	if _, err := c.db.Exec(`
INSERT INTO board_stages (board, key, name, color, position)
VALUES (?, ?, ?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM board_stages WHERE board = ?))`, board.ID, key, name, color, board.ID); err != nil {
		return board, err
	}
	return c.GetBoard(boardRef)
}

// UpdateBoardStage renames or recolors a stage. Its key stays the same.
func (c *Core) UpdateBoardStage(boardRef, stageRef string, in BoardStageInput) (Board, error) {
	board, err := c.boardByKey(c.db, boardRef)
	if err != nil {
		return board, err
	}
	stage, err := c.boardStageByKey(c.db, board, stageRef)
	if err != nil {
		return board, err
	}
	if in.Name != nil {
		if stage.Name, err = cleanBoardName(*in.Name, "stage"); err != nil {
			return board, err
		}
	}
	if in.Color != nil {
		if stage.Color, err = cleanBoardColor(*in.Color); err != nil {
			return board, err
		}
	}
	if _, err := c.db.Exec(`UPDATE board_stages SET name = ?, color = ? WHERE id = ?`, stage.Name, stage.Color, stage.ID); err != nil {
		return board, err
	}
	return c.GetBoard(boardRef)
}

// ReorderBoardStages puts a board's stages in the given order, which must name them all.
func (c *Core) ReorderBoardStages(boardRef string, keys []string) (Board, error) {
	board, err := c.boardByKey(c.db, boardRef)
	if err != nil {
		return board, err
	}
	tx, err := c.db.Beginx()
	if err != nil {
		return board, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.Get(&count, `SELECT COUNT(*) FROM board_stages WHERE board = ?`, board.ID); err != nil {
		return board, err
	}
	seen := map[string]bool{}
	for _, key := range keys {
		seen[key] = true
	}
	if len(keys) != count || len(seen) != count {
		return board, apperr.BadRequest("the order must list every stage once")
	}
	for i, key := range keys {
		res, err := tx.Exec(`UPDATE board_stages SET position = ? WHERE board = ? AND key = ?`, i, board.ID, key)
		if err != nil {
			return board, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return board, err
		} else if n == 0 {
			return board, apperr.BadRequest("the order must list every stage once")
		}
	}
	if err := tx.Commit(); err != nil {
		return board, err
	}
	return c.GetBoard(boardRef)
}

// DeleteBoardStage removes a stage. Its cards move to the end of moveTo, which is required
// if it holds any. A board keeps at least one stage.
func (c *Core) DeleteBoardStage(boardRef, stageRef, moveTo, actor string) (Board, error) {
	board, err := c.boardByKey(c.db, boardRef)
	if err != nil {
		return board, err
	}
	tx, err := c.db.Beginx()
	if err != nil {
		return board, err
	}
	defer tx.Rollback()
	stage, err := c.boardStageByKey(tx, board, stageRef)
	if err != nil {
		return board, err
	}
	var stages, cards int
	if err := tx.Get(&stages, `SELECT COUNT(*) FROM board_stages WHERE board = ?`, board.ID); err != nil {
		return board, err
	}
	if stages <= 1 {
		return board, apperr.BadRequest("a board needs at least one stage")
	}
	if err := tx.Get(&cards, `SELECT COUNT(*) FROM board_cards WHERE stage = ?`, stage.ID); err != nil {
		return board, err
	}
	if cards > 0 {
		if strings.TrimSpace(moveTo) == "" {
			return board, apperr.BadRequest("choose a stage to move this stage's cards to")
		}
		target, err := c.boardStageByKey(tx, board, moveTo)
		if err != nil {
			return board, err
		}
		if target.ID == stage.ID {
			return board, apperr.BadRequest("choose a different stage to move the cards to")
		}
		if _, err := tx.Exec(`
INSERT INTO board_card_events (subscriber, board_name, from_stage_name, to_stage_name, actor_name)
SELECT subscriber, ?, ?, ?, ? FROM board_cards WHERE stage = ?`, board.Name, stage.Name, target.Name, actor, stage.ID); err != nil {
			return board, err
		}
		if _, err := tx.Exec(`
UPDATE board_cards SET stage = ?, moved_at = strftime('%Y-%m-%d %H:%M:%fZ', 'now'),
  position = position + 1 + (SELECT COALESCE(MAX(position), -1) FROM board_cards WHERE stage = ?)
WHERE stage = ?`, target.ID, target.ID, stage.ID); err != nil {
			return board, err
		}
	}
	if _, err := tx.Exec(`DELETE FROM board_stages WHERE id = ?`, stage.ID); err != nil {
		return board, err
	}
	if err := tx.Commit(); err != nil {
		return board, err
	}
	return c.GetBoard(boardRef)
}

type boardCardPlace struct {
	Stage     int64  `db:"stage"`
	StageKey  string `db:"stage_key"`
	StageName string `db:"stage_name"`
}

func (c *Core) boardCardPlace(q sqlx.Queryer, boardID int64, subRecID string) (*boardCardPlace, error) {
	var out boardCardPlace
	err := sqlx.Get(q, &out, `
SELECT c.stage, s.key AS stage_key, s.name AS stage_name
FROM board_cards c JOIN board_stages s ON s.id = c.stage
WHERE c.board = ? AND c.subscriber = ?`, boardID, subRecID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// MoveBoardCard puts a contact in a stage, adding them to the board if they aren't on it.
// A change of stage is recorded in the contact's history. Reordering within a stage isn't.
func (c *Core) MoveBoardCard(in BoardCardMove) (BoardMove, error) {
	out := BoardMove{Subscriber: in.Subscriber}
	if in.Position != nil && *in.Position < 0 {
		return out, apperr.BadRequest("invalid position")
	}
	tx, err := c.db.Beginx()
	if err != nil {
		return out, err
	}
	defer tx.Rollback()

	board, err := c.boardByKey(tx, in.Board)
	if err != nil {
		return out, err
	}
	stage, err := c.boardStageByKey(tx, board, in.Stage)
	if err != nil {
		return out, err
	}
	out.Board, out.BoardName, out.Stage, out.StageName = board.Key, board.Name, stage.Key, stage.Name

	from, err := c.boardCardPlace(tx, board.ID, in.Subscriber)
	if err != nil {
		return out, err
	}
	if from != nil {
		out.PreviousStage, out.PreviousStageName = from.StageKey, from.StageName
	}
	out.Changed = from == nil || from.Stage != stage.ID
	if !out.Changed && in.Position == nil {
		return out, nil
	}

	var movedAt sql.NullString
	if !out.Changed {
		if err := tx.Get(&movedAt, `SELECT moved_at FROM board_cards WHERE board = ? AND subscriber = ?`, board.ID, in.Subscriber); err != nil {
			return out, err
		}
	}
	if _, err := tx.Exec(`DELETE FROM board_cards WHERE board = ? AND subscriber = ?`, board.ID, in.Subscriber); err != nil {
		return out, err
	}
	rank := 0
	if in.Position != nil {
		rank = *in.Position
	}
	position, err := openBoardCardSlot(tx, stage.ID, rank)
	if err != nil {
		return out, err
	}
	if _, err := tx.Exec(`
INSERT INTO board_cards (board, subscriber, stage, position, moved_at)
VALUES (?, ?, ?, ?, COALESCE(?, strftime('%Y-%m-%d %H:%M:%fZ', 'now')))`, board.ID, in.Subscriber, stage.ID, position, movedAt); err != nil {
		return out, err
	}
	if out.Changed {
		if err := recordBoardCardEvent(tx, in.Subscriber, board.Name, out.PreviousStageName, stage.Name, in.Actor); err != nil {
			return out, err
		}
	}
	return out, tx.Commit()
}

// openBoardCardSlot makes room at a rank in a stage and returns the position to use there.
func openBoardCardSlot(tx *sqlx.Tx, stageID int64, rank int) (int, error) {
	var at int
	err := tx.Get(&at, `SELECT position FROM board_cards WHERE stage = ? ORDER BY position, moved_at DESC LIMIT 1 OFFSET ?`, stageID, rank)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.Get(&at, `SELECT COALESCE(MAX(position), -1) + 1 FROM board_cards WHERE stage = ?`, stageID)
		return at, err
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(`UPDATE board_cards SET position = position + 1 WHERE stage = ? AND position >= ?`, stageID, at)
	return at, err
}

func recordBoardCardEvent(tx *sqlx.Tx, subRecID, boardName, from, to, actor string) error {
	_, err := tx.Exec(`
INSERT INTO board_card_events (subscriber, board_name, from_stage_name, to_stage_name, actor_name)
VALUES (?, ?, ?, ?, ?)`, subRecID, boardName, from, to, actor)
	return err
}

// RemoveBoardCard takes a contact off a board. Changed is false if they weren't on it.
func (c *Core) RemoveBoardCard(boardRef, subRecID, actor string) (BoardMove, error) {
	out := BoardMove{Subscriber: subRecID}
	tx, err := c.db.Beginx()
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	board, err := c.boardByKey(tx, boardRef)
	if err != nil {
		return out, err
	}
	out.Board, out.BoardName = board.Key, board.Name
	from, err := c.boardCardPlace(tx, board.ID, subRecID)
	if err != nil || from == nil {
		return out, err
	}
	out.PreviousStage, out.PreviousStageName, out.Changed = from.StageKey, from.StageName, true
	if _, err := tx.Exec(`DELETE FROM board_cards WHERE board = ? AND subscriber = ?`, board.ID, subRecID); err != nil {
		return out, err
	}
	if err := recordBoardCardEvent(tx, subRecID, board.Name, from.StageName, "", actor); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// GetContactBoards lists the boards a contact is on.
func (c *Core) GetContactBoards(subRecID string) ([]ContactBoard, error) {
	out := []ContactBoard{}
	err := c.db.Select(&out, `
SELECT b.key AS board, b.name AS board_name, s.key AS stage, s.name AS stage_name, s.color
FROM board_cards c
JOIN boards b ON b.id = c.board
JOIN board_stages s ON s.id = c.stage
WHERE c.subscriber = ?
ORDER BY b.position, b.id`, subRecID)
	return out, err
}

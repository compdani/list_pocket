package core

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"
)

const (
	ContactTaskOpen = "open"
	ContactTaskDone = "done"

	maxContactNoteChars = 20000
	maxContactTaskChars = 500
)

const crmTimeFormat = "2006-01-02 15:04:05.000Z"

// EnsureCRMTriggers creates the triggers that clear CRM data when a subscriber is
// deleted. Several deletes use raw SQL, so PocketBase cascades don't fire. It runs at
// every boot because rebuilding a collection's table drops its triggers.
func (c *Core) EnsureCRMTriggers() error {
	if _, err := c.db.Exec(`
CREATE TRIGGER IF NOT EXISTS trg_subscribers_crm_cleanup AFTER DELETE ON subscribers
BEGIN
  DELETE FROM contact_profiles WHERE subscriber = OLD.id;
  DELETE FROM contact_notes WHERE subscriber = OLD.id;
  DELETE FROM contact_tasks WHERE subscriber = OLD.id;
  DELETE FROM conversation_state WHERE key = OLD.id;
END;`); err != nil {
		return err
	}
	_, err := c.db.Exec(`
CREATE TRIGGER IF NOT EXISTS trg_subscribers_boards_cleanup AFTER DELETE ON subscribers
BEGIN
  DELETE FROM board_cards WHERE subscriber = OLD.id;
  DELETE FROM board_card_events WHERE subscriber = OLD.id;
END;`)
	return err
}

// ContactProfile holds a contact's CRM fields.
type ContactProfile struct {
	Subscriber string `db:"subscriber" json:"subscriber"`
	Owner      string `db:"owner" json:"owner"`
	OwnerName  string `db:"owner_name" json:"owner_name"`
	Company    string `db:"company" json:"company"`
}

// ContactProfileUpdate changes the fields that aren't nil.
type ContactProfileUpdate struct {
	Owner   *string `json:"owner"`
	Company *string `json:"company"`
}

const crmUserName = `COALESCE(NULLIF(trim(%s.name), ''), %s.username, '')`

func crmUserNameOf(alias string) string {
	return strings.ReplaceAll(crmUserName, "%s", alias)
}

// GetContactProfile returns a contact's CRM fields, which are empty if never set.
func (c *Core) GetContactProfile(subRecID string) (ContactProfile, error) {
	out := ContactProfile{Subscriber: subRecID}
	err := c.db.Get(&out, `
SELECT p.subscriber, p.owner, `+crmUserNameOf("u")+` AS owner_name, p.company
FROM contact_profiles p
LEFT JOIN users u ON u.id = p.owner
WHERE p.subscriber = ?`, subRecID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	return out, err
}

// UpdateContactProfile applies a partial update and returns the result.
func (c *Core) UpdateContactProfile(subRecID string, in ContactProfileUpdate) (ContactProfile, error) {
	cur, err := c.GetContactProfile(subRecID)
	if err != nil {
		return cur, err
	}
	if in.Owner != nil {
		owner := strings.TrimSpace(*in.Owner)
		if err := c.requireCRMUser(owner); err != nil {
			return cur, err
		}
		cur.Owner = owner
	}
	if in.Company != nil {
		cur.Company = strings.TrimSpace(*in.Company)
	}
	if _, err := c.db.Exec(`
INSERT INTO contact_profiles (subscriber, owner, company) VALUES (?, ?, ?)
ON CONFLICT (subscriber) DO UPDATE SET
  owner = excluded.owner, company = excluded.company,
  updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')`,
		subRecID, cur.Owner, cur.Company); err != nil {
		return cur, err
	}
	return c.GetContactProfile(subRecID)
}

func (c *Core) requireCRMUser(userID string) error {
	if userID == "" {
		return nil
	}
	var exists bool
	if err := c.db.Get(&exists, `SELECT EXISTS (SELECT 1 FROM users WHERE id = ?)`, userID); err != nil {
		return err
	}
	if !exists {
		return apperr.BadRequest("unknown user")
	}
	return nil
}

// CRMAssignee is a user who can own contacts and tasks. It carries only what a picker
// shows, because anyone who can read contacts can list these.
type CRMAssignee struct {
	ID   string `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
}

// ListCRMAssignees returns the enabled people who can be given work.
func (c *Core) ListCRMAssignees() ([]CRMAssignee, error) {
	out := []CRMAssignee{}
	err := c.db.Select(&out, `
SELECT u.id, `+crmUserNameOf("u")+` AS name FROM users u
WHERE u.status = 'enabled' AND u.user_type != 'api'
ORDER BY name COLLATE NOCASE`)
	return out, err
}

// ContactNote is a staff note on a contact.
type ContactNote struct {
	ID         int64  `db:"id" json:"id"`
	Subscriber string `db:"subscriber" json:"subscriber"`
	Body       string `db:"body" json:"body"`
	Pinned     bool   `db:"pinned" json:"pinned"`
	Author     string `db:"author" json:"author"`
	AuthorName string `db:"author_name" json:"author_name"`
	Created    string `db:"created" json:"created"`
	Updated    string `db:"updated" json:"updated"`
}

const contactNoteColumns = `id, subscriber, body, pinned, author, author_name, created, updated`

// ListContactNotes returns a contact's notes, pinned first and then newest first.
func (c *Core) ListContactNotes(subRecID string) ([]ContactNote, error) {
	out := []ContactNote{}
	err := c.db.Select(&out, `SELECT `+contactNoteColumns+` FROM contact_notes
WHERE subscriber = ? ORDER BY pinned DESC, datetime(created) DESC, id DESC`, subRecID)
	return out, err
}

func cleanNoteBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", apperr.BadRequest("a note can't be empty")
	}
	if len([]rune(body)) > maxContactNoteChars {
		return "", apperr.BadRequest("that note is too long")
	}
	return body, nil
}

// AddContactNote stores a note. The author's name is kept with it, so the note still
// reads correctly after that user is removed.
func (c *Core) AddContactNote(subRecID, body, authorID, authorName string) (ContactNote, error) {
	var out ContactNote
	body, err := cleanNoteBody(body)
	if err != nil {
		return out, err
	}
	res, err := c.db.Exec(`INSERT INTO contact_notes (subscriber, body, author, author_name) VALUES (?, ?, ?, ?)`,
		subRecID, body, authorID, authorName)
	if err != nil {
		return out, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return out, err
	}
	return c.contactNote(subRecID, id)
}

func (c *Core) contactNote(subRecID string, id int64) (ContactNote, error) {
	var out ContactNote
	err := c.db.Get(&out, `SELECT `+contactNoteColumns+` FROM contact_notes WHERE id = ? AND subscriber = ?`, id, subRecID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, apperr.NotFound("note not found")
	}
	return out, err
}

// UpdateContactNote changes a note's body or pinned state, whichever isn't nil.
func (c *Core) UpdateContactNote(subRecID string, id int64, body *string, pinned *bool) (ContactNote, error) {
	cur, err := c.contactNote(subRecID, id)
	if err != nil {
		return cur, err
	}
	if body != nil {
		if cur.Body, err = cleanNoteBody(*body); err != nil {
			return cur, err
		}
	}
	if pinned != nil {
		cur.Pinned = *pinned
	}
	if _, err := c.db.Exec(`
UPDATE contact_notes SET body = ?, pinned = ?, updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')
WHERE id = ? AND subscriber = ?`, cur.Body, cur.Pinned, id, subRecID); err != nil {
		return cur, err
	}
	return c.contactNote(subRecID, id)
}

// DeleteContactNote removes a note.
func (c *Core) DeleteContactNote(subRecID string, id int64) error {
	if _, err := c.contactNote(subRecID, id); err != nil {
		return err
	}
	_, err := c.db.Exec(`DELETE FROM contact_notes WHERE id = ? AND subscriber = ?`, id, subRecID)
	return err
}

// ContactTask is a follow-up on a contact.
type ContactTask struct {
	ID              int64  `db:"id" json:"id"`
	Subscriber      string `db:"subscriber" json:"subscriber"`
	SubscriberName  string `db:"subscriber_name" json:"subscriber_name"`
	SubscriberEmail string `db:"subscriber_email" json:"subscriber_email"`
	Title           string `db:"title" json:"title"`
	DueAt           string `db:"due_at" json:"due_at"`
	Status          string `db:"status" json:"status"`
	Assignee        string `db:"assignee" json:"assignee"`
	AssigneeName    string `db:"assignee_name" json:"assignee_name"`
	CreatedBy       string `db:"created_by" json:"created_by"`
	Created         string `db:"created" json:"created"`
	CompletedAt     string `db:"completed_at" json:"completed_at"`
}

// ContactTaskInput creates a task, or on update changes the fields that aren't nil.
type ContactTaskInput struct {
	Title    *string `json:"title"`
	DueAt    *string `json:"due_at"`
	Assignee *string `json:"assignee"`
	Status   *string `json:"status"`
}

// ContactTaskQuery filters ListContactTasks. Empty fields don't filter.
type ContactTaskQuery struct {
	Subscriber string
	Assignee   string
	Status     string
	Limit      int
}

const contactTaskSelect = `
SELECT t.id, t.subscriber,
  COALESCE(s.name, '') AS subscriber_name, COALESCE(s.email, '') AS subscriber_email,
  t.title, t.due_at, t.status, t.assignee, ` + `COALESCE(NULLIF(trim(u.name), ''), u.username, '')` + ` AS assignee_name,
  t.created_by, t.created, t.completed_at
FROM contact_tasks t
LEFT JOIN subscribers s ON s.id = t.subscriber
LEFT JOIN users u ON u.id = t.assignee`

// ListContactTasks returns tasks with the soonest due first and undated ones last.
func (c *Core) ListContactTasks(q ContactTaskQuery) ([]ContactTask, error) {
	limit := q.Limit
	if limit < 1 || limit > 500 {
		limit = 200
	}
	var (
		conds []string
		args  []any
	)
	if q.Subscriber != "" {
		conds = append(conds, `t.subscriber = ?`)
		args = append(args, q.Subscriber)
	}
	if q.Assignee != "" {
		conds = append(conds, `t.assignee = ?`)
		args = append(args, q.Assignee)
	}
	if q.Status != "" {
		conds = append(conds, `t.status = ?`)
		args = append(args, q.Status)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	out := []ContactTask{}
	err := c.db.Select(&out, contactTaskSelect+where+`
ORDER BY t.status = 'done', t.due_at = '', datetime(t.due_at), t.id DESC LIMIT ?`, append(args, limit)...)
	return out, err
}

func cleanTaskTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", apperr.BadRequest("a task needs a title")
	}
	if len([]rune(title)) > maxContactTaskChars {
		return "", apperr.BadRequest("that task title is too long")
	}
	return title, nil
}

func cleanTaskDue(due string) (string, error) {
	due = strings.TrimSpace(due)
	if due == "" {
		return "", nil
	}
	t, err := time.Parse(time.RFC3339, due)
	if err != nil {
		return "", apperr.BadRequest("invalid due date")
	}
	return t.UTC().Format(crmTimeFormat), nil
}

// AddContactTask creates an open task.
func (c *Core) AddContactTask(subRecID string, in ContactTaskInput, createdBy string) (ContactTask, error) {
	var (
		out      ContactTask
		title    string
		due      string
		assignee string
		err      error
	)
	if in.Title != nil {
		title = *in.Title
	}
	if title, err = cleanTaskTitle(title); err != nil {
		return out, err
	}
	if in.DueAt != nil {
		if due, err = cleanTaskDue(*in.DueAt); err != nil {
			return out, err
		}
	}
	// A task nobody assigned belongs to whoever created it, so it shows in their list.
	assignee = createdBy
	if in.Assignee != nil {
		assignee = strings.TrimSpace(*in.Assignee)
		if err := c.requireCRMUser(assignee); err != nil {
			return out, err
		}
	}
	res, err := c.db.Exec(`INSERT INTO contact_tasks (subscriber, title, due_at, assignee, created_by) VALUES (?, ?, ?, ?, ?)`,
		subRecID, title, due, assignee, createdBy)
	if err != nil {
		return out, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return out, err
	}
	return c.contactTask(subRecID, id)
}

func (c *Core) contactTask(subRecID string, id int64) (ContactTask, error) {
	var out ContactTask
	err := c.db.Get(&out, contactTaskSelect+` WHERE t.id = ? AND t.subscriber = ?`, id, subRecID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, apperr.NotFound("task not found")
	}
	return out, err
}

// UpdateContactTask applies a partial update. Finishing a task stamps when, and
// reopening one clears that.
func (c *Core) UpdateContactTask(subRecID string, id int64, in ContactTaskInput) (ContactTask, error) {
	cur, err := c.contactTask(subRecID, id)
	if err != nil {
		return cur, err
	}
	if in.Title != nil {
		if cur.Title, err = cleanTaskTitle(*in.Title); err != nil {
			return cur, err
		}
	}
	if in.DueAt != nil {
		if cur.DueAt, err = cleanTaskDue(*in.DueAt); err != nil {
			return cur, err
		}
	}
	if in.Assignee != nil {
		cur.Assignee = strings.TrimSpace(*in.Assignee)
		if err := c.requireCRMUser(cur.Assignee); err != nil {
			return cur, err
		}
	}
	if in.Status != nil {
		switch *in.Status {
		case ContactTaskOpen:
			cur.CompletedAt = ""
		case ContactTaskDone:
			if cur.Status != ContactTaskDone {
				cur.CompletedAt = time.Now().UTC().Format(crmTimeFormat)
			}
		default:
			return cur, apperr.BadRequest("invalid task status")
		}
		cur.Status = *in.Status
	}
	if _, err := c.db.Exec(`
UPDATE contact_tasks SET title = ?, due_at = ?, assignee = ?, status = ?, completed_at = ?
WHERE id = ? AND subscriber = ?`, cur.Title, cur.DueAt, cur.Assignee, cur.Status, cur.CompletedAt, id, subRecID); err != nil {
		return cur, err
	}
	return c.contactTask(subRecID, id)
}

// DeleteContactTask removes a task.
func (c *Core) DeleteContactTask(subRecID string, id int64) error {
	if _, err := c.contactTask(subRecID, id); err != nil {
		return err
	}
	_, err := c.db.Exec(`DELETE FROM contact_tasks WHERE id = ? AND subscriber = ?`, id, subRecID)
	return err
}

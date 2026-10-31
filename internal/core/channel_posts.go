package core

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/compdani/list_pocket/internal/apperr"
)

// Channel post statuses. A post is claimed by moving it from scheduled to sending, and
// it's only ever moved back by a person, because a failed attempt might have posted.
const (
	ChannelPostDraft     = "draft"
	ChannelPostScheduled = "scheduled"
	ChannelPostSending   = "sending"
	ChannelPostSent      = "sent"
	ChannelPostFailed    = "failed"

	channelPostTimeLayout = "2006-01-02 15:04:05"
	channelPostColumns    = `id, channel_id, channel_name, kind, body, media_url, title, send_at, status, provider_msg_id, error, created_by, created, updated`
)

// ChannelPost is one post to a WhatsApp Channel.
type ChannelPost struct {
	ID            int64  `db:"id" json:"id"`
	ChannelID     string `db:"channel_id" json:"channel_id"`
	ChannelName   string `db:"channel_name" json:"channel_name"`
	Kind          string `db:"kind" json:"kind"`
	Body          string `db:"body" json:"body"`
	MediaURL      string `db:"media_url" json:"media_url"`
	Title         string `db:"title" json:"title"`
	SendAt        string `db:"send_at" json:"send_at"`
	Status        string `db:"status" json:"status"`
	ProviderMsgID string `db:"provider_msg_id" json:"provider_msg_id"`
	Error         string `db:"error" json:"error"`
	CreatedBy     string `db:"created_by" json:"created_by"`
	Created       string `db:"created" json:"created"`
	Updated       string `db:"updated" json:"updated"`
}

// ChannelPostInput is what a person fills in. A zero SendAt on a scheduled post means
// as soon as possible.
type ChannelPostInput struct {
	ChannelID   string
	ChannelName string
	Kind        string
	Body        string
	MediaURL    string
	Title       string
	Status      string
	SendAt      time.Time
	CreatedBy   string
}

func channelPostSendAt(in ChannelPostInput) string {
	if in.Status != ChannelPostScheduled {
		return ""
	}
	if in.SendAt.IsZero() {
		return time.Now().UTC().Format(channelPostTimeLayout)
	}
	return in.SendAt.UTC().Format(channelPostTimeLayout)
}

// CreateChannelPost stores a draft or scheduled post.
func (c *Core) CreateChannelPost(in ChannelPostInput) (ChannelPost, error) {
	res, err := c.db.Exec(`
INSERT INTO whatsapp_channel_posts (channel_id, channel_name, kind, body, media_url, title, send_at, status, created_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.ChannelID, in.ChannelName, in.Kind, in.Body, in.MediaURL, in.Title, channelPostSendAt(in), in.Status, in.CreatedBy)
	if err != nil {
		return ChannelPost{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ChannelPost{}, err
	}
	return c.GetChannelPost(id)
}

// GetChannelPost returns one post.
func (c *Core) GetChannelPost(id int64) (ChannelPost, error) {
	var out ChannelPost
	err := c.db.Get(&out, `SELECT `+channelPostColumns+` FROM whatsapp_channel_posts WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return out, apperr.NotFound("post not found")
	}
	return out, err
}

// ListChannelPosts returns the latest posts, newest first.
func (c *Core) ListChannelPosts(limit int) ([]ChannelPost, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	out := []ChannelPost{}
	err := c.db.Select(&out, `SELECT `+channelPostColumns+` FROM whatsapp_channel_posts ORDER BY id DESC LIMIT ?`, limit)
	return out, err
}

// UpdateChannelPost edits a post that hasn't been attempted yet. The status check is in
// the update, so it can't race with the job claiming the post.
func (c *Core) UpdateChannelPost(id int64, in ChannelPostInput) (ChannelPost, error) {
	res, err := c.db.Exec(`
UPDATE whatsapp_channel_posts
SET channel_id = ?, channel_name = ?, kind = ?, body = ?, media_url = ?, title = ?, send_at = ?, status = ?,
    updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')
WHERE id = ? AND status IN (?, ?)`,
		in.ChannelID, in.ChannelName, in.Kind, in.Body, in.MediaURL, in.Title, channelPostSendAt(in), in.Status,
		id, ChannelPostDraft, ChannelPostScheduled)
	if err != nil {
		return ChannelPost{}, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return ChannelPost{}, err
	} else if n == 0 {
		return ChannelPost{}, apperr.New(http.StatusConflict, "only drafts and scheduled posts can be edited")
	}
	return c.GetChannelPost(id)
}

// DeleteChannelPost removes a post that isn't being sent.
func (c *Core) DeleteChannelPost(id int64) error {
	res, err := c.db.Exec(`DELETE FROM whatsapp_channel_posts WHERE id = ? AND status != ?`, id, ChannelPostSending)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return apperr.New(http.StatusConflict, "this post can't be deleted right now")
	}
	return nil
}

// RetryChannelPost schedules a failed post again. It's only ever a person's decision,
// because the failed attempt might have posted.
func (c *Core) RetryChannelPost(id int64) (ChannelPost, error) {
	res, err := c.db.Exec(`
UPDATE whatsapp_channel_posts
SET status = ?, send_at = ?, error = '', updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')
WHERE id = ? AND status = ?`,
		ChannelPostScheduled, time.Now().UTC().Format(channelPostTimeLayout), id, ChannelPostFailed)
	if err != nil {
		return ChannelPost{}, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return ChannelPost{}, err
	} else if n == 0 {
		return ChannelPost{}, apperr.New(http.StatusConflict, "only failed posts can be retried")
	}
	return c.GetChannelPost(id)
}

// CountScheduledChannelPostsOn counts posts on a channel set for the same UTC day, for
// the pacing warning.
func (c *Core) CountScheduledChannelPostsOn(channelID string, day time.Time) (int, error) {
	var n int
	err := c.db.Get(&n, `
SELECT COUNT(*) FROM whatsapp_channel_posts
WHERE channel_id = ? AND status IN (?, ?) AND substr(send_at, 1, 10) = ?`,
		channelID, ChannelPostScheduled, ChannelPostSent, day.UTC().Format("2006-01-02"))
	return n, err
}

// ClaimDueChannelPosts moves due posts to sending and returns those it won. Each claim
// is one atomic update, so overlapping runs can't send the same post.
func (c *Core) ClaimDueChannelPosts(now time.Time) ([]ChannelPost, error) {
	var ids []int64
	if err := c.db.Select(&ids, `
SELECT id FROM whatsapp_channel_posts WHERE status = ? AND send_at != '' AND send_at <= ? ORDER BY send_at, id`,
		ChannelPostScheduled, now.UTC().Format(channelPostTimeLayout)); err != nil {
		return nil, err
	}
	out := make([]ChannelPost, 0, len(ids))
	for _, id := range ids {
		res, err := c.db.Exec(`
UPDATE whatsapp_channel_posts SET status = ?, updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')
WHERE id = ? AND status = ?`, ChannelPostSending, id, ChannelPostScheduled)
		if err != nil {
			return out, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return out, err
		} else if n == 0 {
			continue
		}
		post, err := c.GetChannelPost(id)
		if err != nil {
			return out, err
		}
		out = append(out, post)
	}
	return out, nil
}

// FinishChannelPost records how an attempt ended.
func (c *Core) FinishChannelPost(id int64, status, providerMsgID, message string) error {
	_, err := c.db.Exec(`
UPDATE whatsapp_channel_posts
SET status = ?, provider_msg_id = ?, error = ?, updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')
WHERE id = ?`, status, providerMsgID, message, id)
	return err
}

// FailStuckChannelPosts marks posts left in sending by a crash as failed. The attempt
// might have posted, so a person has to check before retrying.
func (c *Core) FailStuckChannelPosts(olderThan time.Time) error {
	_, err := c.db.Exec(`
UPDATE whatsapp_channel_posts
SET status = ?, error = ?, updated = strftime('%Y-%m-%d %H:%M:%fZ', 'now')
WHERE status = ? AND updated < ?`,
		ChannelPostFailed, "Interrupted while sending. It may have posted, so check the channel before retrying.",
		ChannelPostSending, olderThan.UTC().Format("2006-01-02 15:04:05.000Z"))
	return err
}

package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/core"
)

const (
	channelSuffix   = "@newsletter"
	stuckPostMaxAge = 10 * time.Minute
	postTimeout     = 60 * time.Second

	// PostsPerDayWarning is the top of the range Whapi suggests.
	PostsPerDayWarning = 10
)

// ValidatePost checks a post before it's stored, so it can't fail later for a reason
// a person could have fixed while writing it.
func ValidatePost(p ChannelPost) error {
	if !strings.HasSuffix(p.ChannelID, channelSuffix) {
		return errors.New("choose a channel")
	}
	body := strings.TrimSpace(p.Body)
	switch p.Kind {
	case PostText:
		if body == "" {
			return errors.New("a text post needs text")
		}
	case PostLink:
		if !strings.Contains(body, "https://") && !strings.Contains(body, "http://") {
			return errors.New("a link post needs a URL in its text")
		}
		if strings.TrimSpace(p.Title) == "" {
			return errors.New("a link post needs a title")
		}
	case PostImage, PostVideo:
		parsed, err := url.Parse(p.MediaURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return errors.New("media needs a public https URL")
		}
	default:
		return fmt.Errorf("unknown post type %q", p.Kind)
	}
	return nil
}

// PublishDuePosts sends posts that are due. It's skipped unless the active provider can
// post, which leaves them scheduled. A failed attempt stays failed, because it might
// have posted and a retry could post twice.
func (m *Messenger) PublishDuePosts() {
	provider, err := m.Provider()
	if err != nil || !provider.Capabilities().ChannelPosts {
		return
	}
	now := time.Now()
	if err := m.core.FailStuckChannelPosts(now.Add(-stuckPostMaxAge)); err != nil {
		m.log.Printf("whatsapp posts: %v", err)
	}
	posts, err := m.core.ClaimDueChannelPosts(now)
	if err != nil {
		m.log.Printf("whatsapp posts: claiming: %v", err)
	}
	for _, post := range posts {
		m.publish(provider, post)
	}
}

func (m *Messenger) publish(provider Provider, post core.ChannelPost) {
	ctx, cancel := context.WithTimeout(context.Background(), postTimeout)
	defer cancel()
	result, err := provider.PostToChannel(ctx, ChannelPost{
		ChannelID: post.ChannelID, Kind: post.Kind, Body: post.Body, MediaURL: post.MediaURL, Title: post.Title,
	})
	status, message := core.ChannelPostSent, ""
	if err != nil {
		status, message = core.ChannelPostFailed, err.Error()
		m.log.Printf("whatsapp posts: post %d failed: %v", post.ID, err)
	}
	if err := m.core.FinishChannelPost(post.ID, status, result.MessageID, message); err != nil {
		m.log.Printf("whatsapp posts: recording post %d: %v", post.ID, err)
	}
}

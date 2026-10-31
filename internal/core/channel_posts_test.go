package core

import (
	"sync"
	"testing"
	"time"
)

func TestClaimDueChannelPostsOnlyOnce(t *testing.T) {
	c, _ := channelTestCore(t)
	now := time.Now()
	due, err := c.CreateChannelPost(ChannelPostInput{ChannelID: "1@newsletter", Kind: "text", Body: "due", Status: ChannelPostScheduled, SendAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateChannelPost(ChannelPostInput{ChannelID: "1@newsletter", Kind: "text", Body: "later", Status: ChannelPostScheduled, SendAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateChannelPost(ChannelPostInput{ChannelID: "1@newsletter", Kind: "text", Body: "draft", Status: ChannelPostDraft}); err != nil {
		t.Fatal(err)
	}

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		claims []ChannelPost
	)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := c.ClaimDueChannelPosts(now)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			claims = append(claims, got...)
		}()
	}
	wg.Wait()
	if len(claims) != 1 || claims[0].ID != due.ID || claims[0].Status != ChannelPostSending {
		t.Fatalf("overlapping runs must claim the due post exactly once: %+v", claims)
	}
}

func TestFailedChannelPostOnlyRetriedByHand(t *testing.T) {
	c, _ := channelTestCore(t)
	now := time.Now()
	post, err := c.CreateChannelPost(ChannelPostInput{ChannelID: "1@newsletter", Kind: "text", Body: "x", Status: ChannelPostScheduled, SendAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ClaimDueChannelPosts(now); err != nil {
		t.Fatal(err)
	}
	if err := c.FinishChannelPost(post.ID, ChannelPostFailed, "", "timed out"); err != nil {
		t.Fatal(err)
	}
	again, err := c.ClaimDueChannelPosts(now.Add(time.Hour))
	if err != nil || len(again) != 0 {
		t.Fatalf("a failed post must never be picked up again: %+v %v", again, err)
	}
	if _, err := c.UpdateChannelPost(post.ID, ChannelPostInput{Kind: "text", Status: ChannelPostScheduled}); err == nil {
		t.Fatal("a failed post can't be edited back into the queue")
	}
	retried, err := c.RetryChannelPost(post.ID)
	if err != nil || retried.Status != ChannelPostScheduled || retried.Error != "" {
		t.Fatalf("retry: %+v %v", retried, err)
	}
	if claimed, err := c.ClaimDueChannelPosts(time.Now().Add(time.Minute)); err != nil || len(claimed) != 1 {
		t.Fatalf("a retried post should send: %+v %v", claimed, err)
	}
	if _, err := c.RetryChannelPost(post.ID); err == nil {
		t.Fatal("only failed posts can be retried")
	}
	if err := c.DeleteChannelPost(post.ID); err == nil {
		t.Fatal("a post being sent can't be deleted")
	}
}

func TestStuckChannelPostFails(t *testing.T) {
	c, _ := channelTestCore(t)
	now := time.Now()
	post, err := c.CreateChannelPost(ChannelPostInput{ChannelID: "1@newsletter", Kind: "text", Body: "x", Status: ChannelPostScheduled, SendAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ClaimDueChannelPosts(now); err != nil {
		t.Fatal(err)
	}
	if err := c.FailStuckChannelPosts(now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.GetChannelPost(post.ID); got.Status != ChannelPostSending {
		t.Fatalf("a recent attempt should be left alone: %q", got.Status)
	}
	if err := c.FailStuckChannelPosts(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ := c.GetChannelPost(post.ID)
	if got.Status != ChannelPostFailed || got.Error == "" {
		t.Fatalf("an old attempt should fail with an explanation: %+v", got)
	}
}

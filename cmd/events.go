package main

import (
	pbcore "github.com/pocketbase/pocketbase/core"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/compdani/list_pocket/internal/events"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

// EventStream serves an endpoint that never closes and pushes a
// live event stream (text/event-stream) such as a error messages.
func (a *App) EventStream(re *pbcore.RequestEvent) error {
	hdr := re.Response.Header()
	hdr.Set(echo.HeaderContentType, "text/event-stream")
	hdr.Set(echo.HeaderCacheControl, "no-store")
	hdr.Set(echo.HeaderConnection, "keep-alive")

	// Subscribe to the event stream with a random ID.
	id := fmt.Sprintf("api:%v", time.Now().UnixNano())
	sub, err := a.events.Subscribe(id)
	if err != nil {
		log.Fatalf("error subscribing to events: %v", err)
	}

	ctx := re.Request.Context()
	for {
		select {
		case e := <-sub:
			b, err := json.Marshal(e)
			if err != nil {
				a.log.Printf("error marshalling event: %v", err)
				continue
			}

			re.Response.Write([]byte(fmt.Sprintf("retry: 3000\ndata: %s\n\n", b)))
			flushResponse(re.Response)

		case <-ctx.Done():
			// On HTTP connection close, unsubscribe.
			a.events.Unsubscribe(id)
			return nil
		}
	}

}

func (a *App) publishRealtimeEvent(event events.Event) error {
	if a.pb == nil {
		return nil
	}

	rawData, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := subscriptions.Message{
		Name: "events/" + event.Type,
		Data: rawData,
	}

	for _, client := range a.pb.SubscriptionsBroker().Clients() {
		if client.HasSubscription(msg.Name) {
			client.Send(msg)
		}
	}

	return nil
}

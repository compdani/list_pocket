package main

import (
	"testing"
)

func TestQuoParseMessageWebhookEvent_v3NestedBody(t *testing.T) {
	t.Parallel()
	const raw = `{
  "object": {
    "id": "EVde6b88eff986444fb7b391a62a1dbaeb",
    "object": "event",
    "createdAt": "2026-04-17T22:08:52.983Z",
    "apiVersion": "v3",
    "type": "message.received",
    "data": {
      "object": {
        "id": "AC57f4517f1fa34ea99f3197b1f2636409",
        "object": "message",
        "from": "+12404410745",
        "to": "+17082924025",
        "direction": "incoming",
        "body": "STOP",
        "status": "received",
        "createdAt": "2026-04-17T22:08:52.714Z"
      }
    }
  }
}`
	typ, msg, err := quoParseMessageWebhookEvent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if typ != "message.received" {
		t.Fatalf("type: got %q", typ)
	}
	if msg.From != "+12404410745" || msg.ID != "AC57f4517f1fa34ea99f3197b1f2636409" {
		t.Fatalf("msg: %+v", msg)
	}
	if got := msg.mergedText(); got != "STOP" {
		t.Fatalf("mergedText: got %q want STOP", got)
	}
}

func TestQuoParseMessageWebhookEvent_v4FlatText(t *testing.T) {
	t.Parallel()
	const raw = `{
  "id": "EVc67ec998b35c41d388af50799aeeba3e",
  "object": "event",
  "apiVersion": "v4",
  "createdAt": "2022-01-23T16:55:52.557Z",
  "type": "message.received",
  "data": {
    "object": {
      "id": "AC24a8b8321c4f4cf2be110f4250793d51",
      "object": "message",
      "from": "+19876543210",
      "to": ["+15555555555"],
      "direction": "incoming",
      "text": "Hello, world!",
      "status": "delivered",
      "createdAt": "2022-01-23T16:55:52.420Z"
    }
  }
}`
	typ, msg, err := quoParseMessageWebhookEvent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if typ != "message.received" {
		t.Fatalf("type: got %q", typ)
	}
	if msg.mergedText() != "Hello, world!" {
		t.Fatalf("mergedText: got %q", msg.mergedText())
	}
}

func TestQuoParseMessageWebhookEvent_bodyWinsOverText(t *testing.T) {
	t.Parallel()
	const raw = `{"type":"message.received","data":{"object":{"id":"AC1","from":"+1","to":[],"direction":"incoming","text":"","body":"STOP","status":"received"}}}`
	_, msg, err := quoParseMessageWebhookEvent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if msg.mergedText() != "STOP" {
		t.Fatalf("got %q", msg.mergedText())
	}
}

func TestQuoIsStopKeyword_leadWordsOnly(t *testing.T) {
	t.Parallel()
	if !quoIsStopKeyword("STOP") {
		t.Fatal("expected STOP alone to match")
	}
	if !quoIsStopKeyword("please STOP now") {
		t.Fatal("expected STOP in first four words to match")
	}
	if !quoIsStopKeyword("PLEASE STOP TEXTING ME") {
		t.Fatal("expected STOP within lead words to match")
	}
	long := "👍 to “What does it mean to control real estate but not own it? That’s the topic of DREIA's Main Meeting on 22nd of April at … STOP"
	if quoIsStopKeyword(long) {
		t.Fatal("did not expect STOP buried after many words to match")
	}
	if quoIsStopKeyword("one two three four STOP") {
		t.Fatal("STOP is the 5th word; should not match with limit 4")
	}
	if !quoIsStopKeyword("one two STOP four five six") {
		t.Fatal("STOP is 3rd word; should match")
	}
}

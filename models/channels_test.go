package models

import "testing"

func TestChannelForKeepsExistingBehaviour(t *testing.T) {
	quo := ChannelFor(" quo ")
	if !quo.TextPipeline || !quo.SMSLimits || quo.EmailHeaders || quo.AllowOptin {
		t.Fatalf("unexpected quo pipeline flags: %+v", quo)
	}
	if quo.Address != AddressSourcePhone || quo.Consent != ConsentSourceSMSStatus || quo.Unsendable != UnsendableSMSOptOut {
		t.Fatalf("unexpected quo sources: %+v", quo)
	}
	if quo.TemplateType != TemplateTypeCampaignSMS || !quo.SupportsContentType(CampaignContentTypePlain) || quo.SupportsContentType(CampaignContentTypeHTML) {
		t.Fatalf("unexpected quo content rules: %+v", quo)
	}

	for _, name := range []string{"", "email", "email-work", "my-postback"} {
		s := ChannelFor(name)
		if s.TextPipeline || s.SMSLimits || !s.EmailHeaders || !s.AllowOptin {
			t.Fatalf("%q: unexpected pipeline flags: %+v", name, s)
		}
		if s.Address != AddressSourceEmail || s.Consent != ConsentSourceListStatus || s.Unsendable != UnsendableNone {
			t.Fatalf("%q: unexpected sources: %+v", name, s)
		}
		if s.Disabled {
			t.Fatalf("%q: email-style channel must not be disabled", name)
		}
	}
}

func TestChannelForFailsClosedOnUnregisteredPluginChannel(t *testing.T) {
	s := ChannelFor("ch:unregistered")
	if !s.Disabled || s.Consent != ConsentSourceChannel || s.Address != AddressSourceChannel || s.EmailHeaders {
		t.Fatalf("unregistered plugin channel must fail closed: %+v", s)
	}
}

func TestRegisterChannel(t *testing.T) {
	RegisterChannel(ChannelSpec{ID: "ch:test_reg", Label: "Test", Consent: ConsentSourceChannel, TextPipeline: true})
	t.Cleanup(func() {
		channelsMu.Lock()
		delete(channels, "ch:test_reg")
		channelsMu.Unlock()
	})
	if s := ChannelFor("ch:test_reg"); s.Disabled || s.Label != "Test" {
		t.Fatalf("registered spec not returned: %+v", s)
	}
}

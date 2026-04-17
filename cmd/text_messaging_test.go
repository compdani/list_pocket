package main

import "testing"

func TestQuoIsStopKeyword(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want bool
	}{
		// Exact matches (previously the only supported shapes).
		{"exact stop", "STOP", true},
		{"exact stop lower", "stop", true},
		{"exact unsubscribe", "UNSUBSCRIBE", true},
		{"exact end", "End", true},
		{"exact quit", "quit", true},
		{"exact cancel", "cancel", true},

		// New canonical CTIA keywords.
		{"exact stopall", "stopall", true},
		{"exact optout", "OPTOUT", true},
		{"exact opt-out hyphen", "opt-out", true},
		{"exact revoke", "revoke", true},

		// Punctuation / case variants.
		{"stop with period", "Stop.", true},
		{"stop with bang", "stop!", true},
		{"stop with bangs", "STOP!!!", true},
		{"stop in parens", "(stop)", true},

		// Polite phrasings that real humans send.
		{"stop with trailing word", "stop please", true},
		{"stop texting me", "Stop texting me", true},
		{"please stop", "please stop", true},
		{"please stop punct", "Please, stop.", true},
		{"stop messages lowercase", "stop messaging me", true},
		{"unsubscribe please", "Unsubscribe please", true},
		{"stop multiline", "please\nstop", true},

		// Things that must NOT opt people out.
		{"empty", "", false},
		{"greeting", "hi there", false},
		{"confirm", "YES", false},
		{"no stop word", "more please", false},
		{"substring not word", "nonstop", false},  // "nonstop" should not trigger
		{"substring not word 2", "stopper", false}, // "stopper" should not trigger
		{"endgame not end", "endgame", false},      // "endgame" is one token, not "end"
		{"help", "HELP", false},                    // HELP is a separate keyword, not opt-out
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := quoIsStopKeyword(tc.text)
			if got != tc.want {
				t.Fatalf("quoIsStopKeyword(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestQuoTrimForLog(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"hello", "hello"},
		{"  hello  ", "hello"},
		{"line1\nline2", "line1 line2"},
		{"carriage\r\nreturn", "carriage  return"},
	}
	for _, tc := range cases {
		if got := quoTrimForLog(tc.in); got != tc.want {
			t.Errorf("quoTrimForLog(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	got := quoTrimForLog(string(long))
	if len([]rune(got)) != 141 { // 140 'x' + "…"
		t.Errorf("expected 141-rune truncated output, got %d runes: %q", len([]rune(got)), got)
	}
}

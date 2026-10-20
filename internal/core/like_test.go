package core

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestLikeHelpersMatchLiterally(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cases := []struct {
		value, pattern string
		want           bool
	}{
		{"50% off", likeContains("50%"), true},
		{"500 off", likeContains("50%"), false},
		{"a_b", likeContains("a_b"), true},
		{"axb", likeContains("a_b"), false},
		{`C:\dir`, likeContains(`C:\`), true},
		{"O'Brien", likeContains("o'brien"), true},
		{"Spring sale", likeContains("spring sale"), true},
		{"news_letter", likePrefix("news_"), true},
		{"newsletter", likePrefix("news_"), false},
		{"report_q1", likeSuffix("_q1"), true},
		{"reportxq1", likeSuffix("_q1"), false},
	}
	for _, c := range cases {
		var got bool
		if err := db.Get(&got, `SELECT ? LIKE ?`+likeEsc, c.value, c.pattern); err != nil {
			t.Fatalf("%q LIKE %q: %v", c.value, c.pattern, err)
		}
		if got != c.want {
			t.Errorf("%q LIKE %q = %v, want %v", c.value, c.pattern, got, c.want)
		}
	}
}

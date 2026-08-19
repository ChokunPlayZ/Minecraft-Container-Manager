package servers

import (
	"reflect"
	"testing"
)

func TestParseListOutput(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"There are 2 of a max 20 players online: Steve, Alex", []string{"Steve", "Alex"}},
		{"[Server thread/INFO]: There are 0 of a max 20 players online:", []string{}},
		{"There are 3 of a max 20 players online: a, b, c", []string{"a", "b", "c"}},
		{"[Server thread/INFO]: There are 1 of a max 20 players online: x",
			[]string{"x"}},
		{"nothing relevant here", nil},
	}
	for _, c := range cases {
		got := parseListOutput(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("parseListOutput(%q) = %v want %v", c.in, got, c.want)
		}
	}
}

func TestSplitPlayers(t *testing.T) {
	got := splitPlayers(" Steve,  Alex ,")
	want := []string{"Steve", "Alex"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitPlayers = %v want %v", got, want)
	}
}

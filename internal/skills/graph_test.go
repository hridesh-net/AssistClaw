package skills

import (
	"reflect"
	"testing"
)

func TestResolvePath(t *testing.T) {
	skill := &Skill{
		Nodes: map[string]*Node{
			"A": {Name: "A", Links: []string{"B"}},
			"B": {Name: "B", Links: []string{"C", "D"}},
			"C": {Name: "C", Links: []string{"E"}},
			"D": {Name: "D", Links: []string{"E"}},
			"E": {Name: "E"},
		},
	}

	tests := []struct {
		start, end string
		want       []string
	}{
		{"A", "E", []string{"A", "B", "C", "E"}}, // Shortest (or first found)
		{"B", "E", []string{"B", "C", "E"}},
		{"A", "D", []string{"A", "B", "D"}},
		{"E", "A", nil},
		{"X", "Y", nil},
	}

	for _, tt := range tests {
		got := ResolvePath(skill, tt.start, tt.end)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ResolvePath(%q, %q) = %v; want %v", tt.start, tt.end, got, tt.want)
		}
	}
}

func TestFindBridges(t *testing.T) {
	skill := &Skill{
		Nodes: map[string]*Node{
			"A": {Name: "A", Links: []string{"B"}},
			"B": {Name: "B", Links: []string{"C"}},
			"C": {Name: "C", Links: []string{"D"}},
			"D": {Name: "D"},
		},
	}

	active := []string{"A", "C"}
	// A -> B -> C. Path is [A, B, C]. Bridge is B.
	got := FindBridges(skill, active)
	want := []string{"B"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindBridges(%v) = %v; want %v", active, got, want)
	}

	active = []string{"A", "D"}
	// A -> B -> C -> D. Bridges are B, C.
	got = FindBridges(skill, active)
	if len(got) != 2 {
		t.Errorf("FindBridges(%v) = %v; want 2 bridges", active, got)
	}
}

package simulator

import (
	"reflect"
	"testing"
)

func TestParseTopology(t *testing.T) {
	var cases = []struct {
		connstr  string
		maxFrom  int
		maxTo    int
		expected []*Conn
	}{
		{"1->2", 3, 3, nil},   // Invalid string
		{"c1->2", 3, 3, nil},  // Invalid string
		{"1->s2", 3, 3, nil},  // Invalid string
		{"s1->c2", 3, 3, nil}, // Invalid string

		{"c1->s2", 3, 3, []*Conn{{From: 1, To: 2}}},
		{"c1->s2, c2->s1", 3, 3, []*Conn{{From: 1, To: 2}, {From: 2, To: 1}}},
		{"*->s2, c1->*", 3, 3, []*Conn{
			{From: 0, To: 2}, {From: 1, To: 2}, {From: 2, To: 2},
			{From: 1, To: 0}, {From: 1, To: 1}, {From: 1, To: 2},
		}},
	}
	for _, c := range cases {
		ret := ParseTopology(c.connstr, c.maxFrom, c.maxTo)
		if !reflect.DeepEqual(ret, c.expected) {
			t.Fatalf("Failed to parse topology")
		}
	}
}

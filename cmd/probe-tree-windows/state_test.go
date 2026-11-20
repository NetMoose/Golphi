package main

import "testing"

// Losing a surrogate pair, selection, focus, or caret must reject restoration.
func TestRestoredStateRejectsLostNativeState(t *testing.T) {
	want := editState{Text: "А😀Б é", Start: 1, End: 3, Focus: true, CaretX: 17, CaretY: 2}
	if err := sameState(want, want); err != nil {
		t.Fatal(err)
	}
	cases := []editState{{Text: "АБ é", Start: 1, End: 3, Focus: true, CaretX: 17, CaretY: 2}, {Text: want.Text, Start: 1, End: 2, Focus: true, CaretX: 17, CaretY: 2}, {Text: want.Text, Start: 1, End: 3, CaretX: 17, CaretY: 2}, {Text: want.Text, Start: 1, End: 3, Focus: true, CaretX: 0, CaretY: 2}}
	for _, got := range cases {
		if sameState(want, got) == nil {
			t.Errorf("accepted lost state: %+v", got)
		}
	}
}

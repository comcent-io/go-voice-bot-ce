package chatgpt

import "testing"

func TestSplitDeltaString(t *testing.T) {
	r1, r2, r3 := splitDeltaString("Hello. How are you?")
	e1, e2, e3 := true, "Hello. How are you?", ""

	if r1 != true || r2 != e2 || r3 != e3 {
		t.Errorf("Expected %v, %s, %s but got %v, %s, %s", true, e2, e3, r1, r2, r3)
	}

	r1, r2, r3 = splitDeltaString("Hello. How")
	e1, e2, e3 = true, "Hello.", "How"

	if r1 != e1 || r2 != e2 || r3 != e3 {
		t.Errorf("Expected %v, %s, %s but got %v, %s, %s", e1, e2, e3, r1, r2, r3)
	}

	r1, r2, r3 = splitDeltaString("Hello How")
	e1, e2, e3 = false, "Hello How", ""

	if r1 != e1 || r2 != e2 || r3 != e3 {
		t.Errorf("Expected %v, %s, %s but got %v, %s, %s", e1, e2, e3, r1, r2, r3)
	}

	r1, r2, r3 = splitDeltaString("We")
	e1, e2, e3 = false, "We", ""

	if r1 != e1 || r2 != e2 || r3 != e3 {
		t.Errorf("Expected %v, %s, %s but got %v, %s, %s", e1, e2, e3, r1, r2, r3)
	}

	r1, r2, r3 = splitDeltaString(".We")
	e1, e2, e3 = true, ".", "We"

	if r1 != e1 || r2 != e2 || r3 != e3 {
		t.Errorf("Expected %v, %s, %s but got %v, %s, %s", e1, e2, e3, r1, r2, r3)
	}

	r1, r2, r3 = splitDeltaString("How? It can")
	e1, e2, e3 = true, "How?", "It can"

	if r1 != e1 || r2 != e2 {
		t.Errorf("Expected %s, %s but got %s, %s", e1, e2, r1, r2)
	}

	r1, r2, r3 = splitDeltaString("How! It can")
	e1, e2, e3 = true, "How!", "It can"

	if r1 != e1 || r2 != e2 || r3 != e3 {
		t.Errorf("Expected %v, %s, %s but got %v, %s, %s", e1, e2, e3, r1, r2, r3)
	}
}

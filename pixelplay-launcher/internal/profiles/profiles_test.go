package profiles

import "testing"

func TestList(t *testing.T) {
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("expected profiles")
	}
	t.Logf("profiles=%d first=%s", len(list), list[0].Name)
}

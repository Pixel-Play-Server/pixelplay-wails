package java

import "testing"

func TestMaxCompatibleMajor(t *testing.T) {
	cases := map[int]int{
		8:  8,
		16: 21,
		17: 21,
		21: 21,
		25: 25,
	}
	for req, want := range cases {
		if got := maxCompatibleMajor(req); got != want {
			t.Fatalf("maxCompatibleMajor(%d)=%d want %d", req, got, want)
		}
	}
}

func TestPickJavaPrefersExactRequired(t *testing.T) {
	got, ok := pickJava([]javaCand{
		{path: "/jdk-25/bin/java", major: 25},
		{path: "/zulu_21/bin/java", major: 21},
	}, 21)
	if !ok || got != "/zulu_21/bin/java" {
		t.Fatalf("got %q ok=%v, want zulu 21", got, ok)
	}
}

func TestPickJavaRejectsEmpty(t *testing.T) {
	if _, ok := pickJava(nil, 21); ok {
		t.Fatal("expected no java")
	}
}

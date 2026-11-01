package java

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

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

func TestExtractJDKArchiveZipEvenWithTarGzName(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "jdk-21.tar.gz")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("jdk-21.0.8+9/bin/java.exe")
	_, _ = w.Write([]byte("MZ"))
	_ = zw.Close()
	_ = f.Close()

	dest := filepath.Join(dir, "out")
	if err := extractJDKArchive(src, dest); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "bin", "java.exe")); err != nil {
		t.Fatalf("java.exe missing: %v", err)
	}
}

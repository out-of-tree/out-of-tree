package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectory(t *testing.T) {
	testdir := "test"

	Directory = testdir

	if directory() != testdir {
		t.Fatalf("%s != %s", directory(), testdir)
	}
}

func TestDir(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "out-of-tree_")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmpdir)

	Directory = tmpdir

	for _, testdir := range []string{"a", "a/b", "a/b/c"} {
		expected := filepath.Join(tmpdir, testdir)
		t.Log(testdir, "->", expected)
		resdir := Dir(testdir)
		if resdir != expected {
			t.Fatalf("%s != %s", resdir, expected)
		}

		fi, err := os.Stat(expected)
		if err != nil {
			t.Fatal(err)
		}

		if !fi.IsDir() {
			t.Fatal("not a directory")
		}
	}

	testdir := []string{"a", "b", "c", "d"}
	expected := filepath.Join(append([]string{tmpdir}, testdir...)...)

	t.Log(testdir, "->", expected)
	resdir := Dir(testdir...)
	if resdir != expected {
		t.Fatalf("%s != %s", resdir, expected)
	}

	fi, err := os.Stat(expected)
	if err != nil {
		t.Fatal(err)
	}

	if !fi.IsDir() {
		t.Fatal("not a directory")
	}
}

func TestFile(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "out-of-tree_")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmpdir)

	Directory = tmpdir

	for _, testfile := range []string{"a", "a/b", "a/b/c"} {
		expected := filepath.Join(tmpdir, testfile)
		t.Log(testfile, "->", expected)
		resfile := File(testfile)
		if resfile != expected {
			t.Fatalf("%s != %s", resfile, expected)
		}

		_, err := os.Stat(expected)
		if err == nil {
			t.Fatal("should not exist")
		}

		fi, err := os.Stat(filepath.Dir(expected))
		if err != nil {
			t.Fatal(err)
		}

		if !fi.IsDir() {
			t.Fatal("not a directory")
		}
	}

	testfile := []string{"a", "b", "c"}
	expected := filepath.Join(append([]string{tmpdir}, testfile...)...)
	t.Log(testfile, "->", expected)
	resdir := Dir(testfile...)
	if resdir != expected {
		t.Fatalf("%s != %s", resdir, expected)
	}

	fi, err := os.Stat(expected)
	if err != nil {
		t.Fatal(err)
	}

	if !fi.IsDir() {
		t.Fatal("not a directory")
	}
}

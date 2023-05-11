package snapshot

import (
	"errors"
	"testing"
)

func TestSourcePackageVersions(t *testing.T) {
	versions, err := SourcePackageVersions("linux")
	if err != nil {
		t.Fatal(err)
	}

	if len(versions) == 0 {
		t.Fatal(errors.New("empty response"))
	}

	t.Logf("found %d package versions", len(versions))
}

func TestPackages(t *testing.T) {
	packages, err := Packages("linux", "3.16.5-1", "amd64",
		`^linux-(image|headers)-[0-9\.\-]*-(amd64|amd64-unsigned)$`,
		[]string{})
	if err != nil {
		t.Fatal(err)
	}

	if len(packages) == 0 {
		t.Fatal(errors.New("empty response"))
	}

	t.Log(packages)
}

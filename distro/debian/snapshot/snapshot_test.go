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
	rx := `^(linux-(image|headers)-[a-z+~0-9\.\-]*-(common|amd64|amd64-unsigned)|linux-kbuild-.*)$`

	packages, err := Packages("linux", "5.10.179-1", rx,
		[]string{"amd64", "all"}, []string{})
	if err != nil {
		t.Fatal(err)
	}

	if len(packages) == 0 {
		t.Fatal(errors.New("empty response"))
	}

	for _, pkg := range packages {
		t.Logf("%#v", pkg)
	}
}

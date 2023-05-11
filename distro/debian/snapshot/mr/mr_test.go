package mr

import (
	"testing"
)

func TestMR(t *testing.T) {
	name := "linux"
	t.Log(name)

	pkg, err := GetPackage(name)
	if err != nil {
		t.Fatal(err)
	}

	version := pkg.Result[0].Version
	t.Log(version)

	binpkgs, err := GetBinpackages(name, version)
	if err != nil {
		t.Fatal(err)
	}

	binpkgName := binpkgs.Result[0].Name
	t.Log(binpkgName)

	binary, err := GetBinary(binpkgName)
	if err != nil {
		t.Fatal(err)
	}

	binaryName := binary.Result[0].Name
	binaryVersion := binary.Result[0].BinaryVersion
	t.Log(binaryName, binaryVersion)

	binfiles, err := GetBinfiles(binaryName, binaryVersion)
	if err != nil {
		t.Fatal(err)
	}

	hash := binfiles.Result[0].Hash
	t.Log(hash)

	info, err := GetInfo(hash)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(info)
}

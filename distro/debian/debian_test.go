package debian

import (
	"os"
	"testing"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

func TestMatchImagePkg(t *testing.T) {
	t.Log("tested with cache by default")

	tmp, err := fs.TempDir()
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	config.Directory = tmp

	km := config.KernelMask{
		ReleaseMask:   "3.2.0-4",
		DistroRelease: "7",
	}

	pkgs, err := MatchImagePkg(km)
	if err != nil {
		t.Fatal(err)
	}

	if len(pkgs) == 0 {
		t.Fatal("no packages")
	}
}

func TestParseKernelMajorMinor(t *testing.T) {
	type testcase struct {
		Deb          string
		Major, Minor int
	}

	for _, tc := range []testcase{
		testcase{"linux-image-4.17.0-2-amd64", 4, 17},
		testcase{"linux-image-6.1.0-8-amd64-unsigned", 6, 1},
		testcase{"linux-image-6.1.0-0.deb11.7-amd64-unsigned", 6, 1},
		testcase{"linux-image-3.2.0-0.bpo.4-amd64_3.2.41-2+deb7u2~bpo60+1_amd64", 3, 2},
		testcase{"linux-image-5.16.0-rc5-amd64-unsigned_5.16~rc5-1~exp1_amd64", 5, 16},
		testcase{"linux-image-3.6-trunk-amd64_3.8.4-1~experimental.1_amd64", 3, 6},
	} {
		major, minor, err := parseKernelMajorMinor(tc.Deb)
		if err != nil {
			t.Fatal(err)
		}
		if major != tc.Major || minor != tc.Minor {
			t.Fatalf("%v -> %v.%v != %v.%v", tc.Deb, major, minor,
				tc.Major, tc.Minor)
		}
	}
}

func TestKernelRelease(t *testing.T) {
	type testcase struct {
		Deb     string
		Release Release
	}

	for _, tc := range []testcase{
		testcase{"linux-image-4.17.0-2-amd64", Stretch},
		testcase{"linux-image-6.1.0-8-amd64-unsigned", Bullseye},
		testcase{"linux-image-6.1.0-0.deb11.7-amd64-unsigned", Bullseye},
		testcase{"linux-image-3.2.0-0.bpo.4-amd64_3.2.41-2+deb7u2~bpo60+1_amd64", Wheezy},
		testcase{"linux-image-5.16.0-rc5-amd64-unsigned_5.16~rc5-1~exp1_amd64", Bullseye},
		testcase{"linux-image-3.15-trunk-amd64_3.15.5-1~exp1_amd64", Wheezy},
		testcase{"linux-image-3.16-rc5-amd64_3.16~rc5-1~exp1_amd64", Jessie},
		testcase{"linux-image-4.8.0-2-amd64-unsigned_4.8.15-2_amd64", Jessie},
		testcase{"linux-image-4.9.0-rc3-amd64-unsigned_4.9~rc3-1~exp1_amd64", Stretch},
		testcase{"linux-image-4.15.0-3-amd64_4.15.17-1_amd64", Stretch},
		testcase{"linux-image-4.16.0-rc5-amd64_4.16~rc5-1~exp1_amd64", Stretch},
		testcase{"linux-image-4.18.0-0.bpo.3-amd64_4.18.20-2~bpo9+1_amd64", Stretch},
		testcase{"linux-image-4.19.0-rc2-amd64-unsigned_4.19~rc2-1~exp1_amd64", Buster},
		testcase{"linux-image-4.20.0-trunk-amd64-unsigned_4.20-1~exp1_amd64", Buster},
		testcase{"linux-image-5.0.0-trunk-amd64-unsigned_5.0.1-1~exp1_amd64", Buster},
		testcase{"linux-image-5.9.0-rc4-amd64-unsigned_5.9~rc4-1~exp1_amd64", Buster},
		testcase{"linux-image-5.10.0-rc4-amd64-unsigned_5.10~rc4-1~exp1_amd64", Bullseye},
	} {
		r, err := kernelRelease(tc.Deb)
		if err != nil {
			t.Fatal(err)
		}
		if r != tc.Release {
			t.Fatalf("%v -> %v != %v", tc.Deb, r, tc.Release)
		}
	}
}

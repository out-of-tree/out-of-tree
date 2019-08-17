// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package config

import (
	"testing"

	"github.com/naoina/toml"
)

func TestMarshalUnmarshal(t *testing.T) {
	artifactCfg := Artifact{
		Name: "Put name here",
		Type: KernelModule,
	}
	artifactCfg.SupportedKernels = append(artifactCfg.SupportedKernels,
		KernelMask{Ubuntu, "18.04", ".*", kernel{}})
	buf, err := toml.Marshal(&artifactCfg)
	if err != nil {
		t.Fatal(err)
	}

	var artifactCfgNew Artifact
	err = toml.Unmarshal(buf, &artifactCfgNew)
	if err != nil {
		t.Fatal(err)
	}
}

func TestKernelRegex(t *testing.T) {
	mask := "4[.]4[.]0-(1|2|3|4|5|6|7|8|9|10|11|12|13|14|15|16|17|18|19|20|21|22|23|24|25|26|27|28|29|30|31|32|33|34|35|36|37|38|39|40|41|42|43|44|45|46|47|48|49|50|51|52|53|54|55|56|57|58|59|60|61|62|63|64|65|66|67|68|69|70|71|72|73|74|75|76|77|78|79|80|81|82|83|84|85|86|87|88|89|90|91|92|93|94|95|96|97|98|99|100|101|102|103|104|105|106|107|108|109|110|111|112|113|114|115|116)-.*"
	k := kernel{
		Version: []int{4},
		Major:   []int{4},
		Minor:   []int{0},
		Patch:   []int{1, 116},
	}

	gmask, err := genReleaseMask(k)
	if err != nil {
		t.Fatal(err)
	}

	if mask != gmask {
		t.Fatal("Got", gmask, "instead of", mask)
	}

	mask = "4[.]4[.]0.*"
	k = kernel{
		Version: []int{4},
		Major:   []int{4},
		Minor:   []int{0},
	}

	gmask, err = genReleaseMask(k)
	if err != nil {
		t.Fatal(err)
	}

	if mask != gmask {
		t.Fatal("Got", gmask, "instead of", mask)
	}
}

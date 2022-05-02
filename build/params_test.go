//stm: #unit
package build

import (
	"crypto/md5"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParamsJsonIsValid(t *testing.T) {
	networks := make(map[string]networkParams)
	err := json.Unmarshal(paramsBytes, &networks)
	if err != nil {
		t.Fatal("params json file is invalid", err)
	}
}

func TestGenesisIsCorrect(t *testing.T) {
	sums := map[string][md5.Size]byte{
		"butterflynet.car": {182, 90, 49, 34, 136, 142, 12, 88, 23, 20, 0, 96, 64, 234, 98, 184},
		"calibnet.car":     {168, 37, 124, 119, 3, 121, 163, 120, 81, 73, 13, 192, 217, 176, 38, 106},
		"interopnet.car":   {244, 117, 66, 227, 54, 80, 227, 53, 17, 215, 221, 217, 201, 149, 170, 222},
		"mainnet.car":      {160, 248, 36, 64, 37, 155, 74, 233, 117, 67, 171, 223, 8, 73, 186, 197},
	}
	for k, v := range sums {
		genBytes, err := getGenesisFor(k)
		if err != nil {
			t.Fatal(err)
		}
		sum := md5.Sum(genBytes)
		require.Equal(t, v, sum, "genesis for %s does not match the expected checksum", k)
	}
}

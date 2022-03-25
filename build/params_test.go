//stm: #unit
package build

import (
	"encoding/json"
	"testing"
)

func TestParamsJsonIsValid(t *testing.T) {
	networks := make(map[string]networkParams)
	err := json.Unmarshal(paramsBytes, &networks)
	if err != nil {
		t.Fatal("params json file is invalid", err)
	}
}

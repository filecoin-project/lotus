package jsonfield

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsonBytes(t *testing.T) {
	type dt struct {
		I  int
		St string
	}

	var data dt

	data.I = 20
	data.St = "eouid"

	rawJson, err := json.Marshal(&data)
	require.NoError(t, err)
	require.Equal(t, `{"I":20,"St":"eouid"}`, string(rawJson))

	strFieldJson, err := json.Marshal(&JSONBytes[dt]{data})
	require.NoError(t, err)
	require.Equal(t, `"eyJJIjoyMCwiU3QiOiJlb3VpZCJ9"`, string(strFieldJson))

	var un JSONBytes[dt]
	require.NoError(t, json.Unmarshal(strFieldJson, &un))
	require.Equal(t, 20, un.Data.I)
	require.Equal(t, "eouid", un.Data.St)
}

func TestJsonBytesField(t *testing.T) {
	type dt struct {
		I  int
		St string
	}

	type wrapT struct {
		F JSONBytes[dt]
	}

	var data wrapT

	data.F.Data.I = 20
	data.F.Data.St = "eouid"

	strFieldJson, err := json.Marshal(&data)
	require.NoError(t, err)
	require.Equal(t, `{"F":"eyJJIjoyMCwiU3QiOiJlb3VpZCJ9"}`, string(strFieldJson))

	var un wrapT
	require.NoError(t, json.Unmarshal(strFieldJson, &un))
	require.Equal(t, 20, un.F.Data.I)
	require.Equal(t, "eouid", un.F.Data.St)
}

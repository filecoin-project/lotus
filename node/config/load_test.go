package config

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fullNodeDefault() (interface{}, error) { return DefaultFullNode(), nil }

func TestDecodeNothing(t *testing.T) {
	assert := assert.New(t)

	{
		cfg, err := FromFile(os.DevNull, SetDefault(fullNodeDefault))
		assert.Nil(err, "error should be nil")
		assert.Equal(DefaultFullNode(), cfg,
			"config from empty file should be the same as default")
	}

	{
		cfg, err := FromFile("./does-not-exist.toml", SetDefault(fullNodeDefault))
		assert.Nil(err, "error should be nil")
		assert.Equal(DefaultFullNode(), cfg,
			"config from not existing file should be the same as default")
	}
}

func TestParitalConfig(t *testing.T) {
	assert := assert.New(t)
	cfgString := ` 
		[API]
		Timeout = "10s"
		`
	expected := DefaultFullNode()
	expected.API.Timeout = Duration(10 * time.Second)

	{
		cfg, err := FromReader(bytes.NewReader([]byte(cfgString)), DefaultFullNode())
		assert.NoError(err, "error should be nil")
		assert.Equal(expected, cfg,
			"config from reader should contain changes")
	}

	{
		f, err := os.CreateTemp("", "config-*.toml")
		fname := f.Name()

		assert.NoError(err, "tmp file should not error")
		_, err = f.WriteString(cfgString)
		assert.NoError(err, "writing to tmp file should not error")
		err = f.Close()
		assert.NoError(err, "closing tmp file should not error")
		defer os.Remove(fname) //nolint:errcheck

		cfg, err := FromFile(fname, SetDefault(fullNodeDefault))
		assert.Nil(err, "error should be nil")
		assert.Equal(expected, cfg,
			"config from reader should contain changes")
	}
}

func TestValidateSplitstoreSet(t *testing.T) {
	cfgSet := ` 
		EnableSplitstore = false
		`
	assert.NoError(t, ValidateSplitstoreSet(cfgSet))
	cfgSloppySet := `
		           	EnableSplitstore          =   	true	
	`
	assert.NoError(t, ValidateSplitstoreSet(cfgSloppySet))

	// Missing altogether
	cfgMissing := ` 
	[Chainstore]
	  # type: bool                                                                         
	  # env var: LOTUS_CHAINSTORE_ENABLESPLITSTORE                                         
	  # oops its missing

	[Chainstore.Splitstore]
    	ColdStoreType = "discard"
	`
	err := ValidateSplitstoreSet(cfgMissing)
	assert.Error(t, err)
	cfgCommentedOut := `
				#    EnableSplitstore = false
	`
	err = ValidateSplitstoreSet(cfgCommentedOut)
	assert.Error(t, err)
}

// Default config keeps EnableSplitstore field uncommented
func TestKeepEnableSplitstoreUncommented(t *testing.T) {
	cfgStr, err := ConfigComment(DefaultFullNode())
	assert.NoError(t, err)
	assert.True(t, MatchEnableSplitstoreField(string(cfgStr)))

	cfgStrFromDef, err := ConfigUpdate(DefaultFullNode(), DefaultFullNode(), Commented(true), DefaultKeepUncommented())
	assert.NoError(t, err)
	assert.True(t, MatchEnableSplitstoreField(string(cfgStrFromDef)))
}

// Loading a config with commented EnableSplitstore fails when setting validator
func TestValidateConfigSetsEnableSplitstore(t *testing.T) {
	cfgCommentedOutEnableSS, err := ConfigUpdate(DefaultFullNode(), DefaultFullNode(), Commented(true))
	assert.NoError(t, err)
	// assert that this config comments out EnableSplitstore
	assert.False(t, MatchEnableSplitstoreField(string(cfgCommentedOutEnableSS)))

	// write config with commented out EnableSplitstore to file
	f, err := os.CreateTemp("", "config.toml")
	fname := f.Name()
	assert.NoError(t, err)
	defer func() {
		err = f.Close()
		assert.NoError(t, err)
		os.Remove(fname) //nolint:errcheck
	}()
	_, err = f.WriteString(string(cfgCommentedOutEnableSS))
	assert.NoError(t, err)

	_, err = FromFile(fname, SetDefault(fullNodeDefault), SetValidate(ValidateSplitstoreSet))
	assert.Error(t, err)
}

// Loading without a config file and a default fails if the default fallback is disabled
func TestFailToFallbackToDefault(t *testing.T) {
	dir, err := os.MkdirTemp("", "dirWithNoFiles")
	assert.NoError(t, err)
	defer assert.NoError(t, os.RemoveAll(dir))
	nonExistantFileName := dir + "/notarealfile"
	_, err = FromFile(nonExistantFileName, SetDefault(fullNodeDefault), SetCanFallbackOnDefault(NoDefaultForSplitstoreTransition))
	assert.Error(t, err)
}

func TestPrintDeprecated(t *testing.T) {
	type ChildCfg struct {
		Field    string `moved:"Bang"`
		NewField string
	}
	type Old struct {
		Thing1 int `moved:"New.Thing1"`
		Thing2 int `moved:"New.Thing2"`
	}
	type New struct {
		Thing1 int
		Thing2 int
	}
	type ParentCfg struct {
		Child ChildCfg
		Old   Old
		New   New
		Foo   int
		Baz   string `moved:"Child.NewField"`
		Boom  int    `moved:"Foo"`
		Bang  string
	}

	t.Run("warning output", func(t *testing.T) {
		cfg := `
		Baz = "baz"
		Foo = 100
		[Child]
		Field = "bip"
		NewField = "bop"
	`

		warningWriter := bytes.NewBuffer(nil)

		v, err := FromReader(bytes.NewReader([]byte(cfg)), &ParentCfg{Boom: 200, Bang: "300"}, SetWarningWriter(warningWriter))

		require.NoError(t, err)
		require.Equal(t, &ParentCfg{
			Child: ChildCfg{
				Field:    "bip",
				NewField: "bop",
			},
			Baz:  "baz",
			Foo:  100,
			Boom: 200,
			Bang: "bip",
		}, v)
		require.Regexp(t, `\WChild\.Field\W.+use 'Bang' instead`, warningWriter.String())
		require.Regexp(t, `\WBaz\W.+use 'Child\.NewField' instead`, warningWriter.String())
		require.NotContains(t, warningWriter.String(), "don't use this at all")
		require.NotContains(t, warningWriter.String(), "Boom")
	})

	defaultNew := New{Thing1: 42, Thing2: 800}
	testCases := []struct {
		name     string
		cfg      string
		expected New
	}{
		{"simple", ``, defaultNew},
		{"set new", "[New]\nThing1 = 101\nThing2 = 102\n", New{Thing1: 101, Thing2: 102}},
		// should move old to new fields if new isn't set
		{"set old", "[Old]\nThing1 = 101\nThing2 = 102\n", New{Thing1: 101, Thing2: 102}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := FromReader(bytes.NewReader([]byte(tc.cfg)), &ParentCfg{New: defaultNew})
			require.NoError(t, err)
			require.Equal(t, tc.expected, v.(*ParentCfg).New)
		})
	}
}

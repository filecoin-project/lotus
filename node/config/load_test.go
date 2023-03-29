// stm: #unit
package config

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func fullNodeDefault() (interface{}, error) { return DefaultFullNode(), nil }

func TestDecodeNothing(t *testing.T) {
	//stm: @NODE_CONFIG_LOAD_FILE_002
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
			"config from not exisiting file should be the same as default")
	}
}

func TestParitalConfig(t *testing.T) {
	//stm: @NODE_CONFIG_LOAD_FILE_003
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

		assert.NoError(err, "tmp file shold not error")
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
	  # oops its mising

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

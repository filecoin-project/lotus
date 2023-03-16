package config

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/kelseyhightower/envconfig"
	"golang.org/x/xerrors"
)

// FromFile loads config from a specified file overriding defaults specified in
// the def parameter. If file does not exist or is empty defaults are assumed.
func FromFile(path string, def interface{}, opts ...LoadCfgOpt) (interface{}, error) {
	var loadOpts cfgLoadOpts
	for _, opt := range opts {
		if err := opt(&loadOpts); err != nil {
			return nil, xerrors.Errorf("failed to apply load cfg option: %w", err)
		}
	}

	// check for loadability
	file, err := os.Open(path)
	defer file.Close() //nolint:errcheck,staticcheck // The file is RO
	switch {
	case os.IsNotExist(err):
		if loadOpts.canFallbackOnDefault != nil {
			if err := loadOpts.canFallbackOnDefault(); err != nil {
				return nil, err
			}
		}
		return def, nil
	case err != nil:
		return nil, err
	}

	var buf bytes.Buffer
	tee := io.TeeReader(file, &buf)
	cfgBs, err := ioutil.ReadAll(tee)
	if err != nil {
		return nil, xerrors.Errorf("failed to read config for loadability checks %w", err)
	}
	s := string(cfgBs)
	if err := loadOpts.validate(s); err != nil {
		return nil, xerrors.Errorf("config failed loadability check: %w", err)
	}
	return FromReader(&buf, def)
}

// FromReader loads config from a reader instance.
func FromReader(reader io.Reader, def interface{}) (interface{}, error) {
	cfg := def
	_, err := toml.NewDecoder(reader).Decode(cfg)
	if err != nil {
		return nil, err
	}

	err = envconfig.Process("LOTUS", cfg)
	if err != nil {
		return nil, fmt.Errorf("processing env vars overrides: %s", err)
	}

	return cfg, nil
}

type cfgLoadOpts struct {
	canFallbackOnDefault func() error
	validate             func(string) error
}

type LoadCfgOpt func(opts *cfgLoadOpts) error

func SetCanFallbackOnDefault(f func() error) LoadCfgOpt {
	return func(opts *cfgLoadOpts) error {
		opts.canFallbackOnDefault = f
		return nil
	}
}

func SetValidate(f func(string) error) LoadCfgOpt {
	return func(opts *cfgLoadOpts) error {
		opts.validate = f
		return nil
	}
}

func NoDefaultForSplitstoreTransition() error {
	return xerrors.Errorf("FullNode config not found and fallback to default disallowed while we transition to splitstore discard default.  Use `lotus config default` to set this repo up with a default config.  Be sure to set `EnableSplitstore` to `false` if you are running a full archive node")
}

func ValidateSplitstoreSet(cfgRaw string) error {
	explicitSplitstoreRx := regexp.MustCompile("\n[\t\n\f\r ]*EnableSplitstore =")
	if match := explicitSplitstoreRx.FindString(cfgRaw); match == "" {
		return xerrors.Errorf("Config does not contain explicit set of EnableSplitstore field, refusing to load. Please explicitly set EnableSplitstore. Set it to false if you are running a full archival node")
	}
	return nil
}

type cfgUpdateOpts struct {
	keepUncommented func(string) bool
}

// UpdateCfgOpt is a functional option for updating the config
type UpdateCfgOpt func(opts *cfgUpdateOpts) error

// KeepUncommented sets a function for matching default valeus that should remain uncommented during
// a config update that comments out default values.
func KeepUncommented(f func(string) bool) UpdateCfgOpt {
	return func(opts *cfgUpdateOpts) error {
		opts.keepUncommented = f
		return nil
	}
}

// Match the EnableSplitstore field
func MatchEnableSplitstoreField(s string) bool {
	enableSplitstoreRx := regexp.MustCompile("^[\t\n\f\r ]*EnableSplitstore =")
	match := enableSplitstoreRx.FindString(s)
	return match != ""
}

// ConfigUpdate takes in a config and a default config and optionally comments out default values
func ConfigUpdate(cfgCur, cfgDef interface{}, comment bool, opts ...UpdateCfgOpt) ([]byte, error) {
	var updateOpts cfgUpdateOpts
	for _, opt := range opts {
		if err := opt(&updateOpts); err != nil {
			return nil, xerrors.Errorf("failed to apply update cfg option to ConfigUpdate's config: %w", err)
		}
	}
	var nodeStr, defStr string
	if cfgDef != nil {
		buf := new(bytes.Buffer)
		e := toml.NewEncoder(buf)
		if err := e.Encode(cfgDef); err != nil {
			return nil, xerrors.Errorf("encoding default config: %w", err)
		}

		defStr = buf.String()
	}

	{
		buf := new(bytes.Buffer)
		e := toml.NewEncoder(buf)
		if err := e.Encode(cfgCur); err != nil {
			return nil, xerrors.Errorf("encoding node config: %w", err)
		}

		nodeStr = buf.String()
	}

	if comment {
		// create a map of default lines, so we can comment those out later
		defLines := strings.Split(defStr, "\n")
		defaults := map[string]struct{}{}
		for i := range defLines {
			l := strings.TrimSpace(defLines[i])
			if len(l) == 0 {
				continue
			}
			if l[0] == '#' || l[0] == '[' {
				continue
			}
			defaults[l] = struct{}{}
		}

		nodeLines := strings.Split(nodeStr, "\n")
		var outLines []string

		sectionRx := regexp.MustCompile(`\[(.+)]`)
		var section string

		for i, line := range nodeLines {
			// if this is a section, track it
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 0 {
				if trimmed[0] == '[' {
					m := sectionRx.FindSubmatch([]byte(trimmed))
					if len(m) != 2 {
						return nil, xerrors.Errorf("section didn't match (line %d)", i)
					}
					section = string(m[1])

					// never comment sections
					outLines = append(outLines, line)
					continue
				}
			}

			pad := strings.Repeat(" ", len(line)-len(strings.TrimLeftFunc(line, unicode.IsSpace)))

			// see if we have docs for this field
			{
				lf := strings.Fields(line)
				if len(lf) > 1 {
					doc := findDoc(cfgCur, section, lf[0])

					if doc != nil {
						// found docfield, emit doc comment
						if len(doc.Comment) > 0 {
							for _, docLine := range strings.Split(doc.Comment, "\n") {
								outLines = append(outLines, pad+"# "+docLine)
							}
							outLines = append(outLines, pad+"#")
						}

						outLines = append(outLines, pad+"# type: "+doc.Type)
					}

					outLines = append(outLines, pad+"# env var: LOTUS_"+strings.ToUpper(strings.ReplaceAll(section, ".", "_"))+"_"+strings.ToUpper(lf[0]))
				}
			}

			// filter lines from options
			optsFilter := updateOpts.keepUncommented != nil && updateOpts.keepUncommented(line)
			// if there is the same line in the default config, comment it out it output
			if _, found := defaults[strings.TrimSpace(nodeLines[i])]; (cfgDef == nil || found) && len(line) > 0 && !optsFilter {
				line = pad + "#" + line[len(pad):]
			}
			outLines = append(outLines, line)
			if len(line) > 0 {
				outLines = append(outLines, "")
			}
		}

		nodeStr = strings.Join(outLines, "\n")
	}

	// sanity-check that the updated config parses the same way as the current one
	if cfgDef != nil {
		cfgUpdated, err := FromReader(strings.NewReader(nodeStr), cfgDef)
		if err != nil {
			return nil, xerrors.Errorf("parsing updated config: %w", err)
		}

		if !reflect.DeepEqual(cfgCur, cfgUpdated) {
			return nil, xerrors.Errorf("updated config didn't match current config")
		}
	}

	return []byte(nodeStr), nil
}

func ConfigComment(t interface{}) ([]byte, error) {
	return ConfigUpdate(t, nil, true, KeepUncommented(MatchEnableSplitstoreField))
}

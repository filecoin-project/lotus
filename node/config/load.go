package config

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kelseyhightower/envconfig"
	"golang.org/x/xerrors"
)

// FromFile loads config from a specified file overriding defaults specified in
// the def parameter. If file does not exist or is empty defaults are assumed.
func FromFile(path string, opts ...LoadCfgOpt) (interface{}, error) {
	loadOpts, err := applyOpts(opts...)
	if err != nil {
		return nil, err
	}
	var def interface{}
	if loadOpts.defaultCfg != nil {
		def, err = loadOpts.defaultCfg()
		if err != nil {
			return nil, xerrors.Errorf("no config found")
		}
	}
	// check for loadability
	file, err := os.Open(path)
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
	defer file.Close() //nolint:errcheck,staticcheck // The file is RO
	cfgBs, err := io.ReadAll(file)
	if err != nil {
		return nil, xerrors.Errorf("failed to read config for validation checks %w", err)
	}
	buf := bytes.NewBuffer(cfgBs)
	if loadOpts.validate != nil {
		if err := loadOpts.validate(buf.String()); err != nil {
			return nil, xerrors.Errorf("config failed validation: %w", err)
		}
	}
	return FromReader(buf, def, opts...)
}

// FromReader loads config from a reader instance.
func FromReader(reader io.Reader, def interface{}, opts ...LoadCfgOpt) (interface{}, error) {
	loadOpts, err := applyOpts(opts...)
	if err != nil {
		return nil, err
	}
	cfg := def
	md, err := toml.NewDecoder(reader).Decode(cfg)
	if err != nil {
		return nil, err
	}

	// find any fields with a tag: `moved:"New.Config.Location"` and move any set values there over to
	// the new location if they are not already set there.
	movedFields := findMovedFields(nil, cfg)
	var warningOut io.Writer = os.Stderr
	if loadOpts.warningWriter != nil {
		warningOut = loadOpts.warningWriter
	}
	for _, d := range movedFields {
		if md.IsDefined(d.Field...) {
			_, _ = fmt.Fprintf(
				warningOut,
				"WARNING: Use of deprecated configuration option '%s' will be removed in a future release, use '%s' instead\n",
				strings.Join(d.Field, "."),
				strings.Join(d.NewField, "."))
			if !md.IsDefined(d.NewField...) {
				// new value isn't set but old is, we should move what the user set there
				if err := moveFieldValue(cfg, d.Field, d.NewField); err != nil {
					return nil, fmt.Errorf("failed to move field value: %w", err)
				}
			}
		}
	}

	err = envconfig.Process("LOTUS", cfg)
	if err != nil {
		return nil, fmt.Errorf("processing env vars overrides: %s", err)
	}

	return cfg, nil
}

// move a value from the location in the valPtr struct specified by oldPath, to the location
// specified by newPath; where the path is an array of nested field names.
func moveFieldValue(valPtr interface{}, oldPath []string, newPath []string) error {
	oldValue, err := getFieldValue(valPtr, oldPath)
	if err != nil {
		return err
	}
	val := reflect.ValueOf(valPtr).Elem()
	for {
		field := val.FieldByName(newPath[0])
		if !field.IsValid() {
			return fmt.Errorf("unexpected error fetching field value")
		}
		if len(newPath) == 1 {
			if field.Kind() != oldValue.Kind() {
				return fmt.Errorf("unexpected error, old kind != new kind")
			}
			// set field on val to be the new one, and we're done
			field.Set(oldValue)
			return nil
		}
		if field.Kind() != reflect.Struct {
			return fmt.Errorf("unexpected error fetching field value, is not a struct")
		}
		newPath = newPath[1:]
		val = field
	}
}

// recursively iterate into `path` to find the terminal value
func getFieldValue(val interface{}, path []string) (reflect.Value, error) {
	if reflect.ValueOf(val).Kind() == reflect.Ptr {
		val = reflect.ValueOf(val).Elem().Interface()
	}
	field := reflect.ValueOf(val).FieldByName(path[0])
	if !field.IsValid() {
		return reflect.Value{}, fmt.Errorf("unexpected error fetching field value")
	}
	if len(path) > 1 {
		if field.Kind() != reflect.Struct {
			return reflect.Value{}, fmt.Errorf("unexpected error fetching field value, is not a struct")
		}
		return getFieldValue(field.Interface(), path[1:])
	}
	return field, nil
}

type movedField struct {
	Field    []string
	NewField []string
}

// inspect the fields recursively within a struct and find any with "moved" tags
func findMovedFields(path []string, val interface{}) []movedField {
	dep := make([]movedField, 0)
	if reflect.ValueOf(val).Kind() == reflect.Ptr {
		val = reflect.ValueOf(val).Elem().Interface()
	}
	t := reflect.TypeOf(val)
	if t.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// could also do a "deprecated" in here
		if idx := field.Tag.Get("moved"); idx != "" && idx != "-" {
			dep = append(dep, movedField{
				Field:    append(path, field.Name),
				NewField: strings.Split(idx, "."),
			})
		}
		if field.Type.Kind() == reflect.Struct && reflect.ValueOf(val).FieldByName(field.Name).IsValid() {
			deps := findMovedFields(append(path, field.Name), reflect.ValueOf(val).FieldByName(field.Name).Interface())
			dep = append(dep, deps...)
		}
	}
	return dep
}

type cfgLoadOpts struct {
	defaultCfg           func() (interface{}, error)
	canFallbackOnDefault func() error
	validate             func(string) error
	warningWriter        io.Writer
}

type LoadCfgOpt func(opts *cfgLoadOpts) error

func applyOpts(opts ...LoadCfgOpt) (cfgLoadOpts, error) {
	var loadOpts cfgLoadOpts
	var err error
	for _, opt := range opts {
		if err = opt(&loadOpts); err != nil {
			return loadOpts, fmt.Errorf("failed to apply load cfg option: %w", err)
		}
	}
	return loadOpts, nil
}

func SetDefault(f func() (interface{}, error)) LoadCfgOpt {
	return func(opts *cfgLoadOpts) error {
		opts.defaultCfg = f
		return nil
	}
}

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

func SetWarningWriter(w io.Writer) LoadCfgOpt {
	return func(opts *cfgLoadOpts) error {
		opts.warningWriter = w
		return nil
	}
}

func NoDefaultForSplitstoreTransition() error {
	return xerrors.Errorf("FullNode config not found and fallback to default disallowed while we transition to splitstore discard default.  Use `lotus config default` to set this repo up with a default config.  Be sure to set `EnableSplitstore` to `false` if you are running a full archive node")
}

// MatchEnableSplitstoreField matches the EnableSplitstore field
func MatchEnableSplitstoreField(s string) bool {
	enableSplitstoreRx := regexp.MustCompile(`(?m)^\s*EnableSplitstore\s*=`)
	return enableSplitstoreRx.MatchString(s)
}

func ValidateSplitstoreSet(cfgRaw string) error {
	if !MatchEnableSplitstoreField(cfgRaw) {
		return xerrors.Errorf("Config does not contain explicit set of EnableSplitstore field, refusing to load. Please explicitly set EnableSplitstore. Set it to false if you are running a full archival node")
	}
	return nil
}

type cfgUpdateOpts struct {
	comment         bool
	keepUncommented func(string) bool
	noEnv           bool
}

// UpdateCfgOpt is a functional option for updating the config
type UpdateCfgOpt func(opts *cfgUpdateOpts) error

// KeepUncommented sets a function for matching default values that should remain uncommented during
// a config update that comments out default values.
func KeepUncommented(f func(string) bool) UpdateCfgOpt {
	return func(opts *cfgUpdateOpts) error {
		opts.keepUncommented = f
		return nil
	}
}

func Commented(commented bool) UpdateCfgOpt {
	return func(opts *cfgUpdateOpts) error {
		opts.comment = commented
		return nil
	}
}

func DefaultKeepUncommented() UpdateCfgOpt {
	return KeepUncommented(MatchEnableSplitstoreField)
}

func NoEnv() UpdateCfgOpt {
	return func(opts *cfgUpdateOpts) error {
		opts.noEnv = true
		return nil
	}
}

// ConfigUpdate takes in a config and a default config and optionally comments out default values
func ConfigUpdate(cfgCur, cfgDef interface{}, opts ...UpdateCfgOpt) ([]byte, error) {
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

	if updateOpts.comment {
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

					if !updateOpts.noEnv {
						outLines = append(outLines, pad+"# env var: LOTUS_"+strings.ToUpper(strings.ReplaceAll(section, ".", "_"))+"_"+strings.ToUpper(lf[0]))
					}
				}
			}

			// filter lines from options
			optsFilter := updateOpts.keepUncommented != nil && updateOpts.keepUncommented(line)
			// if there is the same line in the default config, comment it out in output
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

		opts := []cmp.Option{
			// This equality function compares big.Int
			cmpopts.IgnoreUnexported(big.Int{}),
			cmp.Comparer(func(x, y []string) bool {
				tx, ty := reflect.TypeOf(x), reflect.TypeOf(y)
				if tx.Kind() == reflect.Slice && ty.Kind() == reflect.Slice && tx.Elem().Kind() == reflect.String && ty.Elem().Kind() == reflect.String {
					sort.Strings(x)
					sort.Strings(y)
					return strings.Join(x, "\n") == strings.Join(y, "\n")
				}
				return false
			}),
		}

		if !cmp.Equal(cfgUpdated, cfgCur, opts...) {
			return nil, xerrors.Errorf("updated config didn't match current config")
		}
	}

	return []byte(nodeStr), nil
}

func ConfigComment(t interface{}) ([]byte, error) {
	return ConfigUpdate(t, nil, Commented(true), DefaultKeepUncommented())
}

package evmstorage

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"golang.org/x/crypto/sha3"
)

var debugRounds = os.Getenv("LOTUS_EVMSTORAGE_DEBUG") != ""

// Annotation labels one storage slot. Decoded is a scalar for a slot
// holding one variable, or a map of member path to scalar for a packed
// slot.
type Annotation struct {
	Label   string      `json:"label"`
	Type    string      `json:"type,omitempty"`
	Decoded interface{} `json:"decoded,omitempty"`
}

// Hints seed mapping-key probing beyond what the decoder harvests from
// the storage itself.
type Hints struct {
	Ints    []uint64
	Addrs   [][20]byte
	B32s    [][32]byte
	Strings []string
}

// Coverage summarizes how much of a dump the decoder could label.
type Coverage struct {
	Slots      int            `json:"slots"`
	Annotated  int            `json:"annotated"`
	ByVariable map[string]int `json:"byVariable"`
	// TargetsCapped reports that nested-mapping registration hit its
	// safety cap and coverage is lower than the inputs could reach.
	TargetsCapped bool `json:"targetsCapped,omitempty"`
}

// keyList is an append-only candidate set. Targets record how far into
// the list they have probed, so every (target, key) pair is visited
// exactly once across fixpoint rounds without storing misses.
type keyList[K comparable] struct {
	set  map[K]struct{}
	list []K
}

func (l *keyList[K]) add(k K) bool {
	if l.set == nil {
		l.set = make(map[K]struct{})
	}
	if _, ok := l.set[k]; ok {
		return false
	}
	l.set[k] = struct{}{}
	l.list = append(l.list, k)
	return true
}

// mappingTarget is a mapping whose keys can be probed: candidate keys are
// hashed with the base slot and the result looked up in the dump. depth
// counts mapping nesting: a target reached through k probed keys has
// depth k. keyIdx is a watermark into the one key list the target's
// (key type, value type, depth) combination selects.
type mappingTarget struct {
	base    [32]byte
	label   string
	keyType string
	valType string
	depth   int
	// parentInts is the chain of integer keys this target was reached
	// through; intact (len == depth) only when every level was integer-
	// keyed. Intact chains let nested targets inherit sibling evidence.
	parentInts []uint64
	keyIdx     int
	hintIdx    int
	enumDone   bool
}

// chainIntact reports whether every key on the way to this target was an
// integer, so parentInts fully identifies its position.
func (t *mappingTarget) chainIntact() bool {
	return len(t.parentInts) == t.depth
}

// lastParentInt is the immediate outer integer key, when the chain holds.
func (t *mappingTarget) lastParentInt() (uint64, bool) {
	if !t.chainIntact() || len(t.parentInts) == 0 {
		return 0, false
	}
	return t.parentInts[len(t.parentInts)-1], true
}

// chainKey renders an intact chain as a map key.
func chainKey(ints []uint64) string {
	var b strings.Builder
	for _, n := range ints {
		fmt.Fprintf(&b, "%x|", n)
	}
	return b.String()
}

// Decoder labels the slots of one dump using a storage layout. Slot
// positions of mapping entries are keccak-derived and cannot be
// reversed, so the decoder probes forward: it collects candidate keys
// (hints, counters and ids read from already-labelled slots, addresses
// and strings appearing in values) and repeats until a pass labels
// nothing new.
type Decoder struct {
	layout *Layout
	slots  map[[32]byte][32]byte
	ann    map[[32]byte]*Annotation

	targets    []*mappingTarget
	targetSeen map[[32]byte]struct{}

	// Integer candidates in three evidence tiers. ints is everything: the
	// speculative 0..probeRange sweep plus every id-sized value read from
	// a labelled slot; only top-level scalar mappings afford to sweep it.
	// nestedInts is a small bounded set for the inner levels of nested
	// mappings, where per-parent indexes (piece ids, period ids) are
	// dense and small. strongInts hold proof: hints and keys that hit;
	// only they open recursion into mapping-valued mappings, because
	// ... expensive.
	// Deeper recursion (depth >= 2) takes hint keys only.
	ints, nestedInts, strongInts, hintInts keyList[uint64]
	// addresses: candidates are hints, address-typed decoded values and
	// address-shaped raw values; liveAddrs (hints, typed values, hits)
	// open recursion.
	addrs, liveAddrs keyList[[20]byte]
	// strings only ever arrive with evidence (hints, decoded string
	// slots), so one tier suffices.
	strs keyList[string]
	// bytes32 keys are unguessable; hints are their only source.
	b32s keyList[[32]byte]

	// pairInner records proven (outer, inner) integer key pairs: a hit on
	// any nested target under outer key a with inner key b proves the pair
	// exists in the contract's data. Sibling nested mappings indexed by
	// the same outer key consume pairInner[a], which is how a mapping with
	// no directly-probeable content (a doubly-nested string map, say)
	// inherits its key space from a probeable sibling (a keys array).
	pairInner map[uint64][]uint64
	// strCtx scopes string key candidates by the integer-key chain they
	// were harvested under: metadata-keys arrays name the keys of exactly
	// their sibling metadata mapping. Unscoped strings would multiply
	// every string in the contract against every nested string mapping.
	strCtx map[string]*keyList[string]
	// pendingParentInts and pendingChainBroken thread the current probe's
	// key chain to whatever its placement reaches (nested targets,
	// harvested strings); only read/written under mu
	pendingParentInts  []uint64
	pendingChainBroken bool

	// mu serializes all mutation (annotations, key lists, targets).
	// Probing parallelizes across targets: the dominant work is keccak
	// misses against the immutable slots map, which need no lock.
	mu            sync.Mutex
	changed       bool
	targetsCapped bool
}

func NewDecoder(layout *Layout, slots map[[32]byte][32]byte) *Decoder {
	return &Decoder{
		layout:     layout,
		slots:      slots,
		ann:        make(map[[32]byte]*Annotation),
		targetSeen: make(map[[32]byte]struct{}),
		pairInner:  make(map[uint64][]uint64),
		strCtx:     make(map[string]*keyList[string]),
	}
}

// Decode runs the full annotation: static slots, well-known proxy and OZ
// namespaced slots, then mapping probes to a fixpoint. probeRange
// additionally seeds integer keys 0..probeRange for id-keyed mappings.
func (d *Decoder) Decode(hints Hints, probeRange, nestedRange int) map[[32]byte]*Annotation {
	for _, n := range hints.Ints {
		d.ints.add(n)
		d.nestedInts.add(n)
		d.strongInts.add(n)
		d.hintInts.add(n)
	}
	for i := 0; i <= probeRange; i++ {
		d.ints.add(uint64(i))
	}
	for i := 0; i <= min(probeRange, nestedRange); i++ {
		d.nestedInts.add(uint64(i))
	}
	for _, a := range hints.Addrs {
		d.addrs.add(a)
		d.liveAddrs.add(a)
	}
	for _, b := range hints.B32s {
		d.b32s.add(b)
	}
	for _, s := range hints.Strings {
		d.strs.add(s)
	}
	// addresses are worth probing wherever they appear in values
	for _, v := range d.slots {
		if isAddressShaped(v) {
			d.addrs.add([20]byte(v[12:]))
		}
	}

	for _, e := range d.layout.Storage {
		d.place(slotOf(e.slotBig()), e.Offset, e.Label, e.Type, 0)
	}
	d.wellKnown()

	for {
		d.changed = false
		// Snapshots taken between rounds, while nothing is concurrent:
		// workers index these captured slices only, so appends during the
		// round never race and are picked up next round. Only targets with
		// keys past their watermark are scheduled; with hundreds of
		// thousands of nested targets, idle ones must cost nothing.
		keys := roundKeys{
			ints:       d.ints.list,
			nestedInts: d.nestedInts.list,
			strongInts: d.strongInts.list,
			hintInts:   d.hintInts.list,
			addrs:      d.addrs.list,
			liveAddrs:  d.liveAddrs.list,
			b32s:       d.b32s.list,
			strs:       d.strs.list,
		}
		var pending []work
		for _, t := range d.targets {
			if w, ok := d.pendingWork(t, keys); ok {
				pending = append(pending, w)
			}
		}
		if len(pending) == 0 {
			return d.ann
		}
		roundStart := time.Now()
		if debugRounds {
			fmt.Fprintf(os.Stderr, "round: targets=%d pending=%d ints=%d nested=%d strong=%d addrs=%d liveAddrs=%d strs=%d ann=%d\n",
				len(d.targets), len(pending), len(keys.ints), len(keys.nestedInts), len(keys.strongInts), len(keys.addrs), len(keys.liveAddrs), len(keys.strs), len(d.ann))
		}
		var wg sync.WaitGroup
		var next atomic.Int64
		for range runtime.NumCPU() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					i := int(next.Add(1)) - 1
					if i >= len(pending) {
						return
					}
					d.probe(pending[i], keys)
				}
			}()
		}
		wg.Wait()
		if debugRounds {
			fmt.Fprintf(os.Stderr, "  round done in %s\n", time.Since(roundStart))
		}
	}
}

// work is one target scheduled for a probing round, with the evidence-
// derived key slices captured while nothing was concurrent.
type work struct {
	t        *mappingTarget
	pairKeys []uint64
	ctxStrs  []string
}

// pendingWork decides, between rounds and lock-free, whether a target has
// unprobed keys, and captures the key slices the round will use.
func (d *Decoder) pendingWork(t *mappingTarget, keys roundKeys) (work, bool) {
	w := work{t: t}
	keyLabel := ""
	if kt := d.layout.Types[t.keyType]; kt != nil {
		keyLabel = kt.Label
	}
	valType := d.layout.Types[t.valType]
	if valType == nil {
		return w, false
	}
	recursive := valType.Encoding == "mapping"
	switch {
	case strings.HasPrefix(keyLabel, "enum ") || strings.HasPrefix(keyLabel, "bool"):
		return w, !t.enumDone
	case strings.HasPrefix(keyLabel, "uint") || strings.HasPrefix(keyLabel, "int"):
		switch {
		case recursive && t.depth == 0:
			return w, t.keyIdx < len(keys.strongInts)
		case recursive:
			if outer, ok := t.lastParentInt(); ok {
				w.pairKeys = d.pairInner[outer]
			}
			return w, t.keyIdx < len(w.pairKeys) || t.hintIdx < len(keys.hintInts)
		case t.depth == 0:
			return w, t.keyIdx < len(keys.ints)
		default:
			return w, t.keyIdx < len(keys.nestedInts)
		}
	case strings.HasPrefix(keyLabel, "address") || strings.HasPrefix(keyLabel, "contract "):
		if recursive {
			return w, t.keyIdx < len(keys.liveAddrs)
		}
		return w, t.keyIdx < len(keys.addrs)
	case keyLabel == "bytes32":
		return w, t.keyIdx < len(keys.b32s)
	case keyLabel == "string" || keyLabel == "bytes":
		if t.chainIntact() && len(t.parentInts) > 0 {
			if ctx := d.strCtx[chainKey(t.parentInts)]; ctx != nil {
				w.ctxStrs = ctx.list
			}
		}
		return w, t.keyIdx < len(w.ctxStrs) || t.hintIdx < len(keys.strs)
	}
	return w, false
}

// roundKeys freezes the key lists one probing round works over.
type roundKeys struct {
	ints, nestedInts, strongInts, hintInts []uint64
	addrs, liveAddrs                       [][20]byte
	b32s                                   [][32]byte
	strs                                   []string
}

// DefaultNestedProbeRange bounds the speculative integer sweep at the
// inner levels of nested mappings, where a target exists per outer key
// and the full probe range would multiply out. Contracts with few outer
// keys but huge dense inner spaces warrant much larger values.
const DefaultNestedProbeRange = 4096

// maxTargets stops nested-mapping registration running away on
// pathological layouts; annotation degrades instead of the process dying.
const maxTargets = 1 << 20

// Coverage reports label counts by top-level variable name.
func (d *Decoder) Coverage() Coverage {
	cov := Coverage{Slots: len(d.slots), ByVariable: make(map[string]int), TargetsCapped: d.targetsCapped}
	for slot := range d.slots {
		a, ok := d.ann[slot]
		if !ok {
			continue
		}
		cov.Annotated++
		name := a.Label
		if i := strings.IndexAny(name, "[.("); i > 0 {
			name = name[:i]
		}
		cov.ByVariable[name]++
	}
	return cov
}

// place routes one variable at a base slot to its encoding-specific
// annotator. It is used both for top-level layout entries and for
// everything reached from them: struct members, array elements, mapping
// values.
func (d *Decoder) place(base [32]byte, offset int, label, typeName string, depth int) {
	t := d.layout.Types[typeName]
	if t == nil {
		return
	}
	switch t.Encoding {
	case "mapping":
		d.addTarget(&mappingTarget{base: base, label: label, keyType: t.Key, valType: t.Value, depth: depth})
	case "bytes":
		d.placeBytes(base, label, t)
	case "dynamic_array":
		d.placeDynamicArray(base, label, t, depth)
	case "inplace":
		switch {
		case len(t.Members) > 0:
			for _, m := range t.Members {
				d.place(addSlot(base, m.slotBig()), m.Offset, label+"."+m.Label, m.Type, depth)
			}
		case t.Base != "":
			d.placeStaticArray(base, label, t, depth)
		default:
			d.placeScalar(base, offset, label, t)
		}
	}
}

// placeScalar annotates the slot holding one elementary value. Packed
// neighbours accumulate into one annotation keyed by member path.
func (d *Decoder) placeScalar(slot [32]byte, offset int, label string, t *LayoutType) {
	value, ok := d.slots[slot]
	if !ok {
		return
	}
	size := t.size()
	if size < 1 || offset < 0 || offset+size > 32 {
		// a struct member offset from a malformed layout could push the
		// range out of the slot; skip rather than panic
		return
	}
	raw := value[32-offset-size : 32-offset]
	decoded := t.decodeElementary(raw)
	d.harvest(t, raw, decoded)

	a := d.ann[slot]
	if a == nil {
		d.setAnn(slot, &Annotation{Label: label, Type: t.Label, Decoded: decoded})
		return
	}
	// second member landing in the same slot: convert to a packed map
	packed, ok := a.Decoded.(map[string]interface{})
	if !ok {
		packed = map[string]interface{}{a.Label: a.Decoded}
		a.Decoded = packed
		a.Type = "packed"
	}
	if _, dup := packed[label]; dup {
		return
	}
	packed[label] = decoded
	if !strings.Contains(a.Label, label) {
		a.Label += "," + label
	}
	d.changed = true
}

// placeBytes annotates a string/bytes variable: short form is inline
// (even final byte = 2*length), long form keeps 2*length+1 in the slot
// and the content from keccak(slot).
func (d *Decoder) placeBytes(base [32]byte, label string, t *LayoutType) {
	value, ok := d.slots[base]
	if !ok {
		return
	}
	if value[31]%2 == 0 { // short form
		length := int(value[31]) / 2
		d.setAnn(base, &Annotation{Label: label, Type: t.Label, Decoded: renderString(value[:length])})
		d.addString(string(value[:length]))
		return
	}
	length := new(big.Int).SetBytes(value[:])
	length.Sub(length, big.NewInt(1)).Rsh(length, 1)
	d.setAnn(base, &Annotation{Label: label + ".length", Type: t.Label, Decoded: json.Number(length.String())})
	if !length.IsInt64() || length.Int64() > 1<<24 {
		return
	}
	n := length.Int64()
	data := keccakSlot(base[:])
	var content []byte
	for i := int64(0); i*32 < n; i++ {
		s := addSlot(data, big.NewInt(i))
		chunk, ok := d.slots[s]
		if !ok {
			continue
		}
		end := min(int64(32), n-i*32)
		d.setAnn(s, &Annotation{Label: fmt.Sprintf("%s.data[%d]", label, i), Type: t.Label, Decoded: renderString(chunk[:end])})
		content = append(content, chunk[:end]...)
	}
	if int64(len(content)) == n {
		d.addString(string(content))
	}
}

// placeDynamicArray annotates the length slot and, sized by the element
// type, every element slot at keccak(base)+i. Elements smaller than a
// slot pack several per slot; larger ones span consecutive slots.
func (d *Decoder) placeDynamicArray(base [32]byte, label string, t *LayoutType, depth int) {
	value, ok := d.slots[base]
	if !ok {
		return
	}
	length := new(big.Int).SetBytes(value[:])
	d.setAnn(base, &Annotation{Label: label + ".length", Type: t.Label, Decoded: json.Number(length.String())})
	if !length.IsInt64() || length.Int64() > 1<<24 {
		return
	}
	n := length.Int64()
	elem := d.layout.Types[t.Base]
	if elem == nil {
		return
	}
	data := keccakSlot(base[:])
	size := elem.size()
	if size < 1 {
		return
	}
	if elem.isElementary() && size <= 16 {
		per := int64(32 / size)
		for s := int64(0); s*per < n; s++ {
			slot := addSlot(data, big.NewInt(s))
			chunk, ok := d.slots[slot]
			if !ok {
				continue
			}
			vals := make([]interface{}, 0, per)
			for i := int64(0); i < per && s*per+i < n; i++ {
				raw := chunk[32-int(i+1)*size : 32-int(i)*size]
				decoded := elem.decodeElementary(raw)
				d.harvest(elem, raw, decoded)
				vals = append(vals, decoded)
			}
			d.setAnn(slot, &Annotation{
				Label:   fmt.Sprintf("%s[%d..%d]", label, s*per, min(s*per+per, n)-1),
				Type:    elem.Label + "[]",
				Decoded: vals,
			})
		}
		return
	}
	slotsPer := int64((size + 31) / 32)
	for i := int64(0); i < n; i++ {
		d.place(addSlot(data, big.NewInt(i*slotsPer)), 0, fmt.Sprintf("%s[%d]", label, i), t.Base, depth)
	}
}

func (d *Decoder) placeStaticArray(base [32]byte, label string, t *LayoutType, depth int) {
	elem := d.layout.Types[t.Base]
	if elem == nil {
		return
	}
	size := elem.size()
	if size < 1 {
		return
	}
	n := t.size() / size
	if elem.isElementary() && size <= 16 {
		per := 32 / size
		for i := 0; i < n; i++ {
			d.placeScalar(addSlot(base, big.NewInt(int64(i/per))), (i%per)*size, fmt.Sprintf("%s[%d]", label, i), elem)
		}
		return
	}
	slotsPer := int64((size + 31) / 32)
	for i := 0; i < n; i++ {
		d.place(addSlot(base, big.NewInt(int64(i)*slotsPer)), 0, fmt.Sprintf("%s[%d]", label, i), t.Base, depth)
	}
}

// probe advances one mapping target through the key list its key type
// selects, from its watermark to the round's limit. A probe hit promotes
// the key to the live lists, unlocking nested targets next round. Runs
// concurrently with other targets; each target belongs to one goroutine
// per round.
func (d *Decoder) probe(w work, round roundKeys) {
	t := w.t
	valType := d.layout.Types[t.valType]
	if valType == nil {
		return
	}
	keyLabel := ""
	if kt := d.layout.Types[t.keyType]; kt != nil {
		keyLabel = kt.Label
	}
	recursive := valType.Encoding == "mapping"
	intKey := func(k uint64) [32]byte {
		var kb [32]byte
		new(big.Int).SetUint64(k).FillBytes(kb[:])
		return kb
	}
	switch {
	case strings.HasPrefix(keyLabel, "enum ") || strings.HasPrefix(keyLabel, "bool"):
		// one-byte key spaces are cheaper to sweep than to guess
		if t.enumDone {
			return
		}
		t.enumDone = true
		limit := uint64(256)
		if strings.HasPrefix(keyLabel, "bool") {
			limit = 2
		}
		for k := uint64(0); k < limit; k++ {
			kb := intKey(k)
			d.probeKey(t, kb[:], func() string { return strconv.FormatUint(k, 10) }, &k)
		}
	case strings.HasPrefix(keyLabel, "uint") || strings.HasPrefix(keyLabel, "int"):
		probeInt := func(k uint64) {
			kb := intKey(k)
			if d.probeKey(t, kb[:], func() string { return strconv.FormatUint(k, 10) }, &k) {
				d.mu.Lock()
				d.markStrongInt(k)
				if outer, ok := t.lastParentInt(); ok {
					d.pairInner[outer] = append(d.pairInner[outer], k)
					d.changed = true
				}
				d.mu.Unlock()
			}
		}
		var keys []uint64
		switch {
		case recursive && t.depth == 0:
			keys = round.strongInts
		case recursive:
			// inherit the proven key space of probeable siblings under the
			// same outer key, plus explicit hints
			keys = w.pairKeys
			for ; t.hintIdx < len(round.hintInts); t.hintIdx++ {
				probeInt(round.hintInts[t.hintIdx])
			}
		case t.depth == 0:
			keys = round.ints
		default:
			keys = round.nestedInts
		}
		for ; t.keyIdx < len(keys); t.keyIdx++ {
			probeInt(keys[t.keyIdx])
		}
	case strings.HasPrefix(keyLabel, "address") || strings.HasPrefix(keyLabel, "contract "):
		keys := round.addrs
		if recursive {
			keys = round.liveAddrs
		}
		for ; t.keyIdx < len(keys); t.keyIdx++ {
			k := keys[t.keyIdx]
			var kb [32]byte
			copy(kb[12:], k[:])
			if d.probeKey(t, kb[:], func() string { return fmt.Sprintf("0x%x", k) }, nil) {
				d.mu.Lock()
				d.markLiveAddr(k)
				d.mu.Unlock()
			}
		}
	case keyLabel == "bytes32":
		for ; t.keyIdx < len(round.b32s); t.keyIdx++ {
			k := round.b32s[t.keyIdx]
			d.probeKey(t, k[:], func() string { return fmt.Sprintf("0x%x", k) }, nil)
		}
	case keyLabel == "string" || keyLabel == "bytes":
		// chain-scoped evidence first, then hint/static strings
		for ; t.keyIdx < len(w.ctxStrs); t.keyIdx++ {
			k := w.ctxStrs[t.keyIdx]
			d.probeKey(t, []byte(k), func() string { return fmt.Sprintf("%q", k) }, nil)
		}
		for ; t.hintIdx < len(round.strs); t.hintIdx++ {
			k := round.strs[t.hintIdx]
			d.probeKey(t, []byte(k), func() string { return fmt.Sprintf("%q", k) }, nil)
		}
	}
}

// probeKey tests one (mapping, key) pair, annotating whatever it can
// reach and reporting whether it landed anything. Misses are decided
// lock-free against the immutable slots map wherever the value type
// allows, so a miss costs a keccak and a few map lookups; only prospects
// take the mutation lock.
func (d *Decoder) probeKey(t *mappingTarget, keyBytes []byte, keyRepr func() string, parentInt *uint64) bool {
	derived := keccakSlot(keyBytes, t.base[:])
	valType := d.layout.Types[t.valType]
	switch valType.Encoding {
	case "bytes", "dynamic_array":
		// length slot must exist for any content to be reachable
		if _, ok := d.slots[derived]; !ok {
			return false
		}
	case "inplace":
		if valType.isElementary() {
			if _, ok := d.slots[derived]; !ok {
				return false
			}
		} else if len(valType.Members) > 0 {
			// a struct entry occupies consecutive slots; absence of all of
			// them (zero-valued members are unstored) means no entry
			span := min((valType.size()+31)/32, 32)
			found := false
			for i := 0; i < span; i++ {
				if _, ok := d.slots[addSlot(derived, big.NewInt(int64(i)))]; ok {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if parentInt != nil && t.chainIntact() {
		d.pendingParentInts = append(append([]uint64(nil), t.parentInts...), *parentInt)
		d.pendingChainBroken = false
	} else {
		d.pendingParentInts = nil
		d.pendingChainBroken = true
	}
	before := len(d.ann) + len(d.targets)
	d.place(derived, 0, fmt.Sprintf("%s[%s]", t.label, keyRepr()), t.valType, t.depth+1)
	d.pendingParentInts = nil
	d.pendingChainBroken = false
	return len(d.ann)+len(d.targets) > before
}

func (d *Decoder) addTarget(t *mappingTarget) {
	if len(d.targets) >= maxTargets {
		d.targetsCapped = true
		return
	}
	if _, ok := d.targetSeen[t.base]; ok {
		return
	}
	d.targetSeen[t.base] = struct{}{}
	t.parentInts = d.pendingParentInts
	d.targets = append(d.targets, t)
	d.changed = true
}

func (d *Decoder) setAnn(slot [32]byte, a *Annotation) {
	if _, ok := d.slots[slot]; !ok {
		return
	}
	if existing, ok := d.ann[slot]; ok {
		if existing.Label != a.Label && !strings.Contains(existing.Label, a.Label) {
			// first label wins; conflicting derivations are worth surfacing
			existing.Label += " |also| " + a.Label
		}
		return
	}
	d.ann[slot] = a
	d.changed = true
}

// markStrongInt records a proven key, allowed to open top-level
// recursion. Deliberately not added to nestedInts: strong keys are outer
// ids, and every nested target sweeps nestedInts, so it must stay small.
func (d *Decoder) markStrongInt(k uint64) {
	if d.strongInts.add(k) {
		d.ints.add(k)
		d.changed = true
	}
}

func (d *Decoder) markLiveAddr(k [20]byte) {
	if d.liveAddrs.add(k) {
		d.addrs.add(k)
		d.changed = true
	}
}

// addString records a string key candidate. Strings from an intact
// integer-key chain scope to that chain (a keys array names the keys of
// its sibling mapping); strings with no chain (static slots, hints) are
// global; strings under a broken chain are values of string-keyed maps
// and are dropped as key candidates.
func (d *Decoder) addString(s string) {
	switch {
	case d.pendingChainBroken:
	case len(d.pendingParentInts) > 0:
		ck := chainKey(d.pendingParentInts)
		ctx := d.strCtx[ck]
		if ctx == nil {
			ctx = &keyList[string]{}
			d.strCtx[ck] = ctx
		}
		if ctx.add(s) {
			d.changed = true
		}
	default:
		if d.strs.add(s) {
			d.changed = true
		}
	}
}

// harvest feeds decoded values back into the live candidate key sets.
func (d *Decoder) harvest(t *LayoutType, raw []byte, decoded interface{}) {
	switch v := decoded.(type) {
	case json.Number:
		if n, err := strconv.ParseUint(v.String(), 10, 64); err == nil && n <= 1<<32 {
			if d.ints.add(n) {
				d.changed = true
			}
		}
	case string:
		if len(raw) == 20 && (strings.HasPrefix(t.Label, "address") || strings.HasPrefix(t.Label, "contract ")) {
			d.markLiveAddr([20]byte(raw))
		}
	}
}

// ApplyNamespace annotates an ERC-7201 namespaced storage region: the
// base slot comes from the standard's formula over the namespace id, and
// the given layout's slots are treated as relative to it. This is how
// storage that solc deliberately hides from the contract's own layout
// (OZ upgradeable bases and anything else using @custom:storage-location)
// gets labelled; the ids come from those annotations in the source.
func (d *Decoder) ApplyNamespace(id string, l *Layout) {
	// keccak256(abi.encode(uint256(keccak256(id)) - 1)) & ~0xff
	h := keccakSlot([]byte(id))
	n := new(big.Int).SetBytes(h[:])
	n.Sub(n, big.NewInt(1))
	var pre [32]byte
	n.FillBytes(pre[:])
	base := keccakSlot(pre[:])
	base[31] = 0

	// namespace layouts bring their own type tables; solc type names are
	// canonical so first definition wins
	for name, t := range l.Types {
		if _, ok := d.layout.Types[name]; !ok {
			d.layout.Types[name] = t
		}
	}
	for _, e := range l.Storage {
		d.place(addSlot(base, e.slotBig()), e.Offset, id+"."+e.Label, e.Type, 0)
	}
}

// wellKnown annotates storage that no contract layout describes but that
// standard machinery uses: ERC-1967 proxy slots (fixed constants) and the
// OZ v5 upgradeable base contracts' ERC-7201 namespaces. Project-specific
// namespaces come in through ApplyNamespace (--namespace on the CLI);
// these are just the defaults common to most upgradeable contracts.
func (d *Decoder) wellKnown() {
	addrType := &LayoutType{Encoding: "inplace", Label: "address", NumberOfBytes: "20"}

	fixed := func(hexSlot, label string) {
		var slot [32]byte
		b, _ := new(big.Int).SetString(hexSlot, 16)
		b.FillBytes(slot[:])
		if _, ok := d.slots[slot]; ok {
			d.placeScalar(slot, 0, label, addrType)
		}
	}
	// ERC-1967 (keccak("eip1967.proxy.<x>")-1)
	fixed("360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc", "erc1967.implementation")
	fixed("b53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103", "erc1967.admin")
	fixed("a3f0ad74e5423aebfd80d3ef4346578335a9a72aeaee59ff6cb3582b35133d50", "erc1967.beacon")

	for id, l := range defaultNamespaces() {
		d.ApplyNamespace(id, l)
	}
}

// defaultNamespaces is the ERC-7201 storage of the OpenZeppelin v5
// upgradeable bases, expressed as ordinary relative layouts: the same
// shape a --namespace file supplies.
func defaultNamespaces() map[string]*Layout {
	types := map[string]*LayoutType{
		"t_address": {Encoding: "inplace", Label: "address", NumberOfBytes: "20"},
		"t_bytes32": {Encoding: "inplace", Label: "bytes32", NumberOfBytes: "32"},
		"t_uint64":  {Encoding: "inplace", Label: "uint64", NumberOfBytes: "8"},
		"t_uint256": {Encoding: "inplace", Label: "uint256", NumberOfBytes: "32"},
		"t_bool":    {Encoding: "inplace", Label: "bool", NumberOfBytes: "1"},
		"t_string":  {Encoding: "bytes", Label: "string", NumberOfBytes: "32"},
	}
	layout := func(entries ...*LayoutEntry) *Layout {
		return &Layout{Storage: entries, Types: types}
	}
	entry := func(slot json.Number, offset int, label, typ string) *LayoutEntry {
		return &LayoutEntry{Label: label, Offset: offset, Slot: slot, Type: typ}
	}
	return map[string]*Layout{
		"openzeppelin.storage.Ownable": layout(
			entry("0", 0, "_owner", "t_address"),
		),
		"openzeppelin.storage.Initializable": layout(
			entry("0", 0, "_initialized", "t_uint64"),
			entry("0", 8, "_initializing", "t_bool"),
		),
		"openzeppelin.storage.EIP712": layout(
			entry("0", 0, "_hashedName", "t_bytes32"),
			entry("1", 0, "_hashedVersion", "t_bytes32"),
			entry("2", 0, "_name", "t_string"),
			entry("3", 0, "_version", "t_string"),
		),
		"openzeppelin.storage.ReentrancyGuard": layout(
			entry("0", 0, "_status", "t_uint256"),
		),
		"openzeppelin.storage.Pausable": layout(
			entry("0", 0, "_paused", "t_bool"),
		),
	}
}

func slotOf(n *big.Int) [32]byte {
	var out [32]byte
	n.FillBytes(out[:])
	return out
}

func addSlot(base [32]byte, i *big.Int) [32]byte {
	n := new(big.Int).SetBytes(base[:])
	n.Add(n, i)
	var out [32]byte
	n.FillBytes(out[:])
	return out
}

func keccakSlot(parts ...[]byte) [32]byte {
	h := sha3.NewLegacyKeccak256()
	for _, p := range parts {
		h.Write(p)
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// isAddressShaped filters values worth trying as address keys: 20 bytes
// under 12 zero bytes, with entropy in the high half so small integers do
// not qualify. Masked ID addresses (0xff...) pass.
func isAddressShaped(v [32]byte) bool {
	for _, b := range v[:12] {
		if b != 0 {
			return false
		}
	}
	for _, b := range v[12:22] {
		if b != 0 {
			return true
		}
	}
	return false
}

// renderString shows printable text as text, anything else as hex.
func renderString(b []byte) interface{} {
	s := string(b)
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return fmt.Sprintf("0x%x", b)
		}
	}
	return s
}

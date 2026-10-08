package build

import (
	"archive/tar"
	"context"
	"embed"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

//go:embed actors/*.tar.zst
var embeddedBuiltinActorReleases embed.FS

func BuggyBuiltinActorsMetadataForNetwork(network string, version actorstypes.Version) *BuiltinActorsMetadata {
	for _, m := range buggyBuiltinActorsMetadata {
		if m.Network == network && m.Version == version {
			return m
		}
	}
	return nil
}

func init() {
	if BundleOverrides == nil {
		BundleOverrides = make(map[actorstypes.Version]string)
	}
	for _, av := range actors.Versions {
		path := os.Getenv(fmt.Sprintf("LOTUS_BUILTIN_ACTORS_V%d_BUNDLE", av))
		if path == "" {
			continue
		}
		BundleOverrides[actorstypes.Version(av)] = path
	}
	if err := loadManifests(buildconstants.NetworkBundle); err != nil {
		panic(err)
	}

	setupBuggyActorsMeta()
}

func setupBuggyActorsMeta() {
	if buildconstants.NetworkBundle == "calibrationnet" {
		// The following code cid existed temporarily on the calibnet testnet, as a "buggy" storage miner actor implementation.
		// We include them in our builtin bundle, but intentionally omit from metadata.
		actors.AddActorMeta("storageminer", cid.MustParse("bafk2bzacecnh2ouohmonvebq7uughh4h3ppmg4cjsk74dzxlbbtlcij4xbzxq"), actorstypes.Version12)
		actors.AddActorMeta("storageminer", cid.MustParse("bafk2bzaced7emkbbnrewv5uvrokxpf5tlm4jslu2jsv77ofw2yqdglg657uie"), actorstypes.Version12)
		actors.AddActorMeta("verifiedregistry", cid.MustParse("bafk2bzacednskl3bykz5qpo54z2j2p4q44t5of4ktd6vs6ymmg2zebsbxazkm"), actorstypes.Version13)
	}

	for _, m := range buggyBuiltinActorsMetadata {
		if m.Network == buildconstants.NetworkBundle {
			for name, c := range m.Actors {
				actors.AddActorMeta(name, c, m.Version)
			}
		}
	}
}

// UseNetworkBundle switches to a different network bundle, by name.
func UseNetworkBundle(netw string) error {
	if buildconstants.NetworkBundle == netw {
		return nil
	}
	if err := loadManifests(netw); err != nil {
		return err
	}
	buildconstants.NetworkBundle = netw
	setupBuggyActorsMeta()
	return nil
}

func loadManifests(netw string) error {
	overridden := make(map[actorstypes.Version]struct{})
	var newMetadata []*BuiltinActorsMetadata
	// First, prefer overrides.
	for av, path := range BundleOverrides {
		root, actorCids, err := readBundleManifestFromFile(path)
		if err != nil {
			return err
		}
		newMetadata = append(newMetadata, &BuiltinActorsMetadata{
			Network:     netw,
			Version:     av,
			ManifestCid: root,
			Actors:      actorCids,
		})
		overridden[av] = struct{}{}
	}

	// Then load embedded bundle metadata.
	for _, meta := range EmbeddedBuiltinActorsMetadata {
		if meta.Network != netw {
			continue
		}
		if _, ok := overridden[meta.Version]; ok {
			continue
		}
		newMetadata = append(newMetadata, meta)
	}

	actors.ClearManifests()

	for _, meta := range newMetadata {
		actors.RegisterManifest(meta.Version, meta.ManifestCid, meta.Actors)
	}

	return nil
}

type BuiltinActorsMetadata struct {
	Network      string
	Version      actorstypes.Version
	ManifestCid  cid.Cid
	Actors       map[string]cid.Cid
	BundleGitTag string
}

// ReadEmbeddedBuiltinActorsMetadata reads the metadata from the embedded built-in actor bundles.
// There should be no need to call this method as the result is cached in the
// `EmbeddedBuiltinActorsMetadata` variable on `make gen`.
func ReadEmbeddedBuiltinActorsMetadata() ([]*BuiltinActorsMetadata, error) {
	files, err := embeddedBuiltinActorReleases.ReadDir("actors")
	if err != nil {
		return nil, xerrors.Errorf("failed to read embedded bundle directory: %s", err)
	}
	var bundles []*BuiltinActorsMetadata
	for _, dirent := range files {
		name := dirent.Name()
		b, err := readEmbeddedBuiltinActorsMetadata(name)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, b...)
	}
	// Sort by network, then by bundle.
	sort.Slice(bundles, func(i, j int) bool {
		if bundles[i].Network == bundles[j].Network {
			return bundles[i].Version < bundles[j].Version
		}
		return bundles[i].Network < bundles[j].Network
	})
	return bundles, nil
}

func readEmbeddedBuiltinActorsMetadata(bundle string) ([]*BuiltinActorsMetadata, error) {
	const (
		archiveExt   = ".tar.zst"
		bundleExt    = ".car"
		bundlePrefix = "builtin-actors-"
	)

	if !strings.HasPrefix(bundle, "v") {
		return nil, xerrors.Errorf("bundle '%q' doesn't start with a 'v'", bundle)
	}
	if !strings.HasSuffix(bundle, archiveExt) {
		return nil, xerrors.Errorf("bundle '%q' doesn't end with '%s'", bundle, archiveExt)
	}
	version, err := strconv.ParseInt(bundle[1:len(bundle)-len(archiveExt)], 10, 0)
	if err != nil {
		return nil, xerrors.Errorf("failed to parse actors version from bundle '%q': %s", bundle, err)
	}
	fi, err := embeddedBuiltinActorReleases.Open(fmt.Sprintf("actors/%s", bundle))
	if err != nil {
		return nil, err
	}
	defer fi.Close() //nolint

	uncompressed, err := zstd.NewReader(fi)
	if err != nil {
		return nil, err
	}
	defer uncompressed.Close() //nolint

	var bundles []*BuiltinActorsMetadata

	tarReader := tar.NewReader(uncompressed)
	for {
		header, err := tarReader.Next()
		switch err {
		case io.EOF:
			return bundles, nil
		case nil:
		default:
			return nil, err
		}

		// Read the network name from the bundle name.
		name := path.Base(header.Name)
		if !strings.HasSuffix(name, bundleExt) {
			return nil, xerrors.Errorf("expected bundle to end with .car: %s", name)
		}
		if !strings.HasPrefix(name, bundlePrefix) {
			return nil, xerrors.Errorf("expected bundle to end with .car: %s", name)
		}
		name = name[len(bundlePrefix) : len(name)-len(bundleExt)]

		// Load the bundle.
		root, actorCids, err := readBundleManifest(tarReader)
		if err != nil {
			return nil, xerrors.Errorf("error loading builtin actors bundle: %w", err)
		}

		// The following manifest cids existed temporarily on the calibnet testnet
		// We include them in our builtin bundle, but intentionally omit from metadata
		if root == cid.MustParse("bafy2bzacedrunxfqta5skb7q7x32lnp4efz2oq7fn226ffm7fu5iqs62jkmvs") ||
			root == cid.MustParse("bafy2bzacebl4w5ptfvuw6746w7ev562idkbf5ppq72e6zub22435ws2rukzru") ||
			root == cid.MustParse("bafy2bzacea4firkyvt2zzdwqjrws5pyeluaesh6uaid246tommayr4337xpmi") {
			continue
		}
		// Check our buggy manifests and exclude this one if it's listed there.
		var isBuggyManifest bool
		for _, m := range buggyBuiltinActorsMetadata {
			if root == m.ManifestCid {
				isBuggyManifest = true
			}
		}
		if isBuggyManifest {
			continue
		}
		bundles = append(bundles, &BuiltinActorsMetadata{
			Network:     name,
			Version:     actorstypes.Version(version),
			ManifestCid: root,
			Actors:      actorCids,
		})
	}
}

func readBundleManifestFromFile(path string) (cid.Cid, map[string]cid.Cid, error) {
	fi, err := os.Open(path)
	if err != nil {
		return cid.Undef, nil, err
	}
	defer fi.Close() //nolint

	return readBundleManifest(fi)
}

func readBundleManifest(r io.Reader) (cid.Cid, map[string]cid.Cid, error) {
	// Load the bundle.
	bs := blockstore.NewMemory()
	hdr, err := car.LoadCar(context.Background(), bs, r)
	if err != nil {
		return cid.Undef, nil, xerrors.Errorf("error loading builtin actors bundle: %w", err)
	}

	if len(hdr.Roots) != 1 {
		return cid.Undef, nil, xerrors.Errorf("expected one root when loading actors bundle, got %d", len(hdr.Roots))
	}
	root := hdr.Roots[0]
	actorCids, err := actors.ReadManifest(context.Background(), adt.WrapStore(context.Background(), cbor.NewCborStore(bs)), root)
	if err != nil {
		return cid.Undef, nil, err
	}

	// Make sure we have all the actors.
	for name, c := range actorCids {
		if has, err := bs.Has(context.Background(), c); err != nil {
			return cid.Undef, nil, xerrors.Errorf("got an error when checking that the bundle has the actor %q: %w", name, err)
		} else if !has {
			return cid.Undef, nil, xerrors.Errorf("actor %q missing from bundle", name)
		}
	}

	return root, actorCids, nil
}

// GetEmbeddedBuiltinActorsBundle returns the builtin-actors bundle for the given actors version.
func GetEmbeddedBuiltinActorsBundle(version actorstypes.Version, networkBundleName string) ([]byte, bool) {
	fi, err := embeddedBuiltinActorReleases.Open(fmt.Sprintf("actors/v%d.tar.zst", version))
	if err != nil {
		return nil, false
	}
	defer fi.Close() //nolint

	uncompressed, err := zstd.NewReader(fi)
	if err != nil {
		return nil, false
	}
	defer uncompressed.Close() //nolint

	tarReader := tar.NewReader(uncompressed)
	targetFileName := fmt.Sprintf("builtin-actors-%s.car", networkBundleName)
	for {
		header, err := tarReader.Next()
		switch err {
		case io.EOF:
			return nil, false
		case nil:
		default:
			panic(err)
		}
		if header.Name != targetFileName {
			continue
		}

		car, err := io.ReadAll(tarReader)
		if err != nil {
			panic(err)
		}
		return car, true
	}
}

var buggyBuiltinActorsMetadata = []*BuiltinActorsMetadata{
	{
		// superseded by v16.0.1, included here for testing
		Network:      "butterflynet",
		Version:      16,
		BundleGitTag: "v16.0.0",
		ManifestCid:  cid.MustParse("bafy2bzacedn2h6huw7v2elmmdmpv4phdv4wjwgct7kcrrtdgz7jkjdm6uwa6k"),
		Actors: map[string]cid.Cid{
			"account":          cid.MustParse("bafk2bzacebtd4bl5htqzhnueqwchxjocw35bjfk4u34h33tkhl7l3ll7jjzp6"),
			"cron":             cid.MustParse("bafk2bzacebuwt5zc2njcsm2fghlrnz3bfvu7s6hepl6leoql3w7zwxtfz5sx4"),
			"datacap":          cid.MustParse("bafk2bzacearpj5e5mgj7wbqknzbr67z464mscraxa2r434i4vf2q5pfxli67y"),
			"eam":              cid.MustParse("bafk2bzaceak462ytr36seexcpydhsxywzm3sz3loechzuuuifvtie72zgggkm"),
			"ethaccount":       cid.MustParse("bafk2bzacecosm2oq5ymatrxkhfv722fawci6bfe5x2hz4gkzu7ewakc5cqfsi"),
			"evm":              cid.MustParse("bafk2bzacebqxdknf6fbuqnop7jacz4rtarsji4xp6x4o46sabkdauckekpzvs"),
			"init":             cid.MustParse("bafk2bzaceanzpajkcik5k4xt7nisqiazme2saivwl5um2que2g7jekr6feygm"),
			"multisig":         cid.MustParse("bafk2bzaceayq4iq5l324rlxhrluetxr2rhkw653ple52qdqcc3pxnxgqfobky"),
			"paymentchannel":   cid.MustParse("bafk2bzaceaifoxdlegtehjeow4sve6zuvvd6yqhwpwaim65hexv2gu7dt32ru"),
			"placeholder":      cid.MustParse("bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro"),
			"reward":           cid.MustParse("bafk2bzacecj4o5ea74dksbshkqf4yd2e4wipcnn6es5h4ltvdnmod3wrj7pei"),
			"storagemarket":    cid.MustParse("bafk2bzacedjnmezgiaszypr2pvwrzuzijv6hoegdcz47yq3ym6rcguqw6altm"),
			"storageminer":     cid.MustParse("bafk2bzacebervdatmrxkhke2z7vsmbw4bdcvimxfrvo75lchiobtuyhvklrq6"),
			"storagepower":     cid.MustParse("bafk2bzacecq27y2fgtst57klr5g22nn6i25icey7uq6ofhouupmidkcoh34xa"),
			"system":           cid.MustParse("bafk2bzaceb2l6ttyv3hnjexdzqaoo7nxsec3dtbo5h5yxdzz44ugq2onxofdu"),
			"verifiedregistry": cid.MustParse("bafk2bzacedhrjtvtmygawmtfukkufpw5ju7wasxnfktxloc77gz3nj6mh45b2"),
		},
	},
	{
		// superseded by v16.0.1, included here because it was deployed live and needed before the fix upgrade
		Network:      "calibrationnet",
		Version:      16,
		BundleGitTag: "v16.0.0",
		ManifestCid:  cid.MustParse("bafy2bzacebc7zpsrihpyd2jdcvmegbbk6yhzkifre3hxtoul5wdxxklbwitry"),
		Actors: map[string]cid.Cid{
			"account":          cid.MustParse("bafk2bzaced4jhgt6peqc3m2lrclj347kjhwt3wjsdxrbt336m5kxcrkyyfg4o"),
			"cron":             cid.MustParse("bafk2bzacecuvdunhtjo52tpxt4ge3ue7mpejmv3c3loqf6iafu5e2jlybvy5a"),
			"datacap":          cid.MustParse("bafk2bzacebs5hd67p3x2ohf357xaebz57o3ffeuexndaptn5g3usgatd32icq"),
			"eam":              cid.MustParse("bafk2bzacedbazvsncva5hfb72jyipsebzlo6sgjbfnf6m4p4xhagzoekzgy34"),
			"ethaccount":       cid.MustParse("bafk2bzacedepzmyi2sbw7fgblhzhz75oovy6trbsfnxsbsqdh6te5cchgptiq"),
			"evm":              cid.MustParse("bafk2bzacedomvviwbdddcfm73uaedqeyuiyswdt3plq3v74uvbo2xvrzyphio"),
			"init":             cid.MustParse("bafk2bzacednq5wfauimq4shz2xynzshatk54odj45gp6fkw77gy25fpmhu5oc"),
			"multisig":         cid.MustParse("bafk2bzaceb4bfxfccm5v6qnecp7ayalitk2fvu6ezavcrd7lcb4uohrsaeo32"),
			"paymentchannel":   cid.MustParse("bafk2bzaceaia5ufr2wyzbasrx6vcfeqjsnaqajmlrnp763cesa7pqvddaubyu"),
			"placeholder":      cid.MustParse("bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro"),
			"reward":           cid.MustParse("bafk2bzacecgorusdavjd42ktijbjh4veu2y6isnlfyys2f5jylnc2c6yll3ju"),
			"storagemarket":    cid.MustParse("bafk2bzacea63rezmai4qwvzlc3hmcjn4eurkcec7cjqoih6vztnkwjvvlx2we"),
			"storageminer":     cid.MustParse("bafk2bzaceax4mv3wzp7jfjpsp2lklujzpjtskpn6lsn743a6ljpgzo2qjdncq"),
			"storagepower":     cid.MustParse("bafk2bzaceddedx24uzsyx6yh63aoabx66caaariopsa5gzn2x6yme7dv5s7cg"),
			"system":           cid.MustParse("bafk2bzacecfol5vcebbl7dqkat7kt65bgqwqvn7fxnakg7qvpokce6ulemo3k"),
			"verifiedregistry": cid.MustParse("bafk2bzacedqpwyprkgwdbcqahgrzuoul42gd3hvgn54fxyqjgtwmmlquycuok"),
		},
	},
	{
		// superseded by v16.0.1, included here for testing
		Network:      "devnet",
		Version:      16,
		BundleGitTag: "v16.0.0",
		ManifestCid:  cid.MustParse("bafy2bzaceafzrqb6adck3o3mbyd33gtp2272f577hidqyo7cszg2ksn5sebh2"),
		Actors: map[string]cid.Cid{
			"account":          cid.MustParse("bafk2bzacedqy7umr3bmmkrepi3ostdxrlxaln33wecqllsz5eqgdwzb2oqov2"),
			"cron":             cid.MustParse("bafk2bzacebrbwcwi3jtplab7jajkccgcqzyvpobypfhomf45ujxpgwehipoia"),
			"datacap":          cid.MustParse("bafk2bzaceccxwbelrowdo47xfgd7ldj7trvh7qvq5xgnhxdnw2zdlml5ahzvc"),
			"eam":              cid.MustParse("bafk2bzacebjzmdd7vdjfdrrxzzyu34gz4jzropcyzrc7og4lt2mczipwoz3pc"),
			"ethaccount":       cid.MustParse("bafk2bzaceb7zgpsi6mwc7asrpxyj7zd6rcsuhraekbonodegxhuli2mwdmuw2"),
			"evm":              cid.MustParse("bafk2bzacea5shmgqycbwuvazd2fika4erfbbyxomsnircps2nqk2o7oxtkvdy"),
			"init":             cid.MustParse("bafk2bzacedoyhv5wergunblh2ga65rxbwowhtac4vdz7woovkponmmd55f244"),
			"multisig":         cid.MustParse("bafk2bzacedyid3docv7i2wx4sciua64ndrbfqbk3werhuruehedhj6gj6hvdg"),
			"paymentchannel":   cid.MustParse("bafk2bzaceab556e24d4qzptdownttepcrgljrfvheykwf2kwxzomk5gip3qka"),
			"placeholder":      cid.MustParse("bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro"),
			"reward":           cid.MustParse("bafk2bzacecusrwhrdsndyokbordb7khms3mjvkohnzxst6a3intb52skg56vw"),
			"storagemarket":    cid.MustParse("bafk2bzaceciw5btcu55jguj7yfpgd3ecrwlu7hwhdacbug56htiseitbohw6m"),
			"storageminer":     cid.MustParse("bafk2bzacead3ptx4vq2e7x3q6lm5cpe6lbtp44shxon23xhmahvvfjqbotpny"),
			"storagepower":     cid.MustParse("bafk2bzacecxjuml3buylyyg6u2k6dy3ek4apqlbwbk4npgosxc2cls7ywosow"),
			"system":           cid.MustParse("bafk2bzaceaonvsrizf3fvy2tgruqjgymtfvkzejmooui2w2stpi7teibtz4wm"),
			"verifiedregistry": cid.MustParse("bafk2bzaceb3qajgx4ichpmgnxjeofv3pg3qwagrsgkimoi6sinjnd6mydiy2o"),
		},
	},
	{
		Network:      "testing",
		Version:      16,
		BundleGitTag: "v16.0.0",
		ManifestCid:  cid.MustParse("bafy2bzacea35za4a3ljlupfn2wxdzwz5py6vbu267s5p4p4uvdoqyhhggbbfq"),
		Actors: map[string]cid.Cid{
			"account":          cid.MustParse("bafk2bzacedzuxjcmialu22l24x6z5x2j3lxsxxruxmhvhpkttkvlkrjulkbyq"),
			"cron":             cid.MustParse("bafk2bzacebgi4wwp2dr4z4wtyziz4mn7ipglpez2ghxzh76sbapz4isvxfbc4"),
			"datacap":          cid.MustParse("bafk2bzacechuwtorkaqcmfgztggv5khbx36xyqw5emxibh6qd7si4pdg6i22q"),
			"eam":              cid.MustParse("bafk2bzaceddukui4pbnrfwcu7zv5acvbhgvclblxiwph2utpndb7qqj3oll5o"),
			"ethaccount":       cid.MustParse("bafk2bzaceafucqy3me7ellhrd5jla2a5wqo652agx4nb6467po643zgvy5d4w"),
			"evm":              cid.MustParse("bafk2bzacedmaykyptdfzwkbui6ptoueqsoj3wubtexdeumh7i7qsnlfmreetm"),
			"init":             cid.MustParse("bafk2bzacecauzck4cmqcbjzsz7jyugig5xcqhuj4lrkbykewkg3pineperxg6"),
			"multisig":         cid.MustParse("bafk2bzaceakkpute3br5t77lkfh4qi7ezzafrpoen7xngf46zjvfuaok3ffqi"),
			"paymentchannel":   cid.MustParse("bafk2bzacebwedmnkgpxta4y4pc3fjn5dazcjmlveztmv7gh6czi7zkrx6t4do"),
			"placeholder":      cid.MustParse("bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro"),
			"reward":           cid.MustParse("bafk2bzaceac3ohau7sxi3xotwsi5bzo75lg74nwdmrxmx5rfnmkp7mo26uxsk"),
			"storagemarket":    cid.MustParse("bafk2bzacebn5ixsfxu36r2dbtvfp4ineu4dnhsvqm4qcpiph4cc6knqqkthto"),
			"storageminer":     cid.MustParse("bafk2bzacec3alndcdemx26bl3svrbcxtxpdjnmsfuhrawhx5ldd2dlfzzldq4"),
			"storagepower":     cid.MustParse("bafk2bzaceb4iqnikgmwuqlzzpb3lfl5k4glt32vgfxebu7h4r6zpzmyaxol7i"),
			"system":           cid.MustParse("bafk2bzaceajnotcpt6sx2afno7fjppp6wzrh4dgu2meeoptcjkorunvshi2hw"),
			"verifiedregistry": cid.MustParse("bafk2bzaceazuqbqpozviewg7obtnhlq6tfewjl5vp5wdvkjxdlqbpkvig3zfu"),
		},
	},
	{
		Network:      "testing-fake-proofs",
		Version:      16,
		BundleGitTag: "v16.0.0",
		ManifestCid:  cid.MustParse("bafy2bzacec4tth5r62ny75wt27tyzgzc4nc3zte6cxainlrxs54g7yvmlezs4"),
		Actors: map[string]cid.Cid{
			"account":          cid.MustParse("bafk2bzacedzuxjcmialu22l24x6z5x2j3lxsxxruxmhvhpkttkvlkrjulkbyq"),
			"cron":             cid.MustParse("bafk2bzacebgi4wwp2dr4z4wtyziz4mn7ipglpez2ghxzh76sbapz4isvxfbc4"),
			"datacap":          cid.MustParse("bafk2bzacechuwtorkaqcmfgztggv5khbx36xyqw5emxibh6qd7si4pdg6i22q"),
			"eam":              cid.MustParse("bafk2bzaceddukui4pbnrfwcu7zv5acvbhgvclblxiwph2utpndb7qqj3oll5o"),
			"ethaccount":       cid.MustParse("bafk2bzaceafucqy3me7ellhrd5jla2a5wqo652agx4nb6467po643zgvy5d4w"),
			"evm":              cid.MustParse("bafk2bzacedmaykyptdfzwkbui6ptoueqsoj3wubtexdeumh7i7qsnlfmreetm"),
			"init":             cid.MustParse("bafk2bzacecauzck4cmqcbjzsz7jyugig5xcqhuj4lrkbykewkg3pineperxg6"),
			"multisig":         cid.MustParse("bafk2bzaceai472rp5ba3xputcmaoqlleasxifzgjcnmthcnqpmgb24no2tdhk"),
			"paymentchannel":   cid.MustParse("bafk2bzacebwedmnkgpxta4y4pc3fjn5dazcjmlveztmv7gh6czi7zkrx6t4do"),
			"placeholder":      cid.MustParse("bafk2bzacedfvut2myeleyq67fljcrw4kkmn5pb5dpyozovj7jpoez5irnc3ro"),
			"reward":           cid.MustParse("bafk2bzaceac3ohau7sxi3xotwsi5bzo75lg74nwdmrxmx5rfnmkp7mo26uxsk"),
			"storagemarket":    cid.MustParse("bafk2bzacebn5ixsfxu36r2dbtvfp4ineu4dnhsvqm4qcpiph4cc6knqqkthto"),
			"storageminer":     cid.MustParse("bafk2bzaceaajmyyjy6tntcxngtc55nero4a4halejajpmeioub6fyznbqni74"),
			"storagepower":     cid.MustParse("bafk2bzaceb4iqnikgmwuqlzzpb3lfl5k4glt32vgfxebu7h4r6zpzmyaxol7i"),
			"system":           cid.MustParse("bafk2bzaceajnotcpt6sx2afno7fjppp6wzrh4dgu2meeoptcjkorunvshi2hw"),
			"verifiedregistry": cid.MustParse("bafk2bzaceazuqbqpozviewg7obtnhlq6tfewjl5vp5wdvkjxdlqbpkvig3zfu"),
		},
	},
}

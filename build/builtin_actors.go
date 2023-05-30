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

	"github.com/DataDog/zstd"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

//go:embed actors/*.tar.zst
var embeddedBuiltinActorReleases embed.FS

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
	if err := loadManifests(NetworkBundle); err != nil {
		panic(err)
	}
}

// UseNetworkBundle switches to a different network bundle, by name.
func UseNetworkBundle(netw string) error {
	if NetworkBundle == netw {
		return nil
	}
	if err := loadManifests(netw); err != nil {
		return err
	}
	NetworkBundle = netw
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
		return nil, xerrors.Errorf("bundle bundle '%q' doesn't start with a 'v'", bundle)
	}
	if !strings.HasSuffix(bundle, archiveExt) {
		return nil, xerrors.Errorf("bundle bundle '%q' doesn't end with '%s'", bundle, archiveExt)
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

	uncompressed := zstd.NewReader(fi)
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
func GetEmbeddedBuiltinActorsBundle(version actorstypes.Version) ([]byte, bool) {
	fi, err := embeddedBuiltinActorReleases.Open(fmt.Sprintf("actors/v%d.tar.zst", version))
	if err != nil {
		return nil, false
	}
	defer fi.Close() //nolint

	uncompressed := zstd.NewReader(fi)
	defer uncompressed.Close() //nolint

	tarReader := tar.NewReader(uncompressed)
	targetFileName := fmt.Sprintf("builtin-actors-%s.car", NetworkBundle)
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

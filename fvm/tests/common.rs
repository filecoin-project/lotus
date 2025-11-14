use anyhow::Result;
use cid::Cid;
use fvm::machine::Manifest;
use fvm_integration_tests::bundle::import_bundle;
use fvm_integration_tests::tester::{BasicAccount, BasicTester, ExecutionOptions, Tester};
use fvm_ipld_blockstore::{Blockstore, MemoryBlockstore};
use fvm_ipld_encoding::CborStore;
use fvm_shared::address::Address;
use fvm_shared::econ::TokenAmount;
use fvm_shared::state::StateTreeVersion;
use fvm_shared::version::NetworkVersion;
use multihash_codetable::Code;

// Embedded actor bundle from builtin-actors (dev-dependency `actors`).
use actors; // fil_builtin_actors_bundle

// Minimal EthAccount state view mirroring kernel expectations.
#[derive(fvm_ipld_encoding::tuple::Serialize_tuple)]
pub struct EthAccountStateView {
    pub delegate_to: Option<[u8; 20]>,
    pub auth_nonce: u64,
    pub evm_storage_root: Cid,
}

pub struct Harness {
    pub tester: BasicTester,
    pub ethaccount_code: Cid,
    pub bundle_root: Cid,
}

pub fn new_harness(options: ExecutionOptions) -> Result<Harness> {
    // Build a blockstore and import the embedded bundle.
    let bs = MemoryBlockstore::default();
    let root = import_bundle(&bs, actors::BUNDLE_CAR)?;
    // Load manifest to fetch EthAccount code.
    let (ver, data_root): (u32, Cid) = bs
        .get_cbor(&root)?
        .expect("bundle manifest header not found");
    let manifest = Manifest::load(&bs, &data_root, ver)?;
    let ethaccount_code = *manifest.get_ethaccount_code();

    // Initialize a tester with this bundle.
    let mut tester = Tester::new(NetworkVersion::V21, StateTreeVersion::V5, root, bs)?;
    tester.options = Some(options);

    Ok(Harness { tester, ethaccount_code, bundle_root: root })
}

/// Create an EthAccount actor with the given authority delegated f4 address and EVM delegate (20 bytes).
/// Returns the assigned ActorID of the authority account.
pub fn set_ethaccount_with_delegate(
    h: &mut Harness,
    authority_addr: Address,
    delegate20: [u8; 20],
)
-> Result<u64> {
    // Register the authority address to obtain an ActorID.
    let state_tree = h
        .tester
        .state_tree
        .as_mut()
        .expect("state tree should be present prior to instantiation");
    let authority_id = state_tree.register_new_address(&authority_addr).unwrap();

    // Persist minimal EthAccount state.
    let view = EthAccountStateView { delegate_to: Some(delegate20), auth_nonce: 0, evm_storage_root: Cid::default() };
    let st_cid = state_tree.store().put_cbor(&view, Code::Blake2b256)?;

    // Install the EthAccount actor state with delegated_address = authority_addr.
    let act = fvm::state_tree::ActorState::new(h.ethaccount_code, st_cid, TokenAmount::default(), 0, Some(authority_addr));
    state_tree.set_actor(authority_id, act);
    Ok(authority_id)
}

/// Lookup a code CID by name from the bundle manifest (e.g., "evm").
pub fn bundle_code_by_name(h: &Harness, name: &str) -> Result<Option<Cid>> {
    let store = h.tester.state_tree.as_ref().unwrap().store();
    // bundle_root encodes (manifest_version, manifest_data_cid)
    let (ver, data_root): (u32, Cid) = store.get_cbor(&h.bundle_root)?.expect("bundle header");
    if ver != 1 { return Ok(None); }
    let entries: Vec<(String, Cid)> = store.get_cbor(&data_root)?.expect("manifest data");
    Ok(entries.into_iter().find(|(n, _)| n == name).map(|(_, c)| c))
}

/// Pre-install an EVM actor at a specific f4 address with provided runtime bytecode.
/// Returns the assigned ActorID.
pub fn install_evm_contract_at(
    h: &mut Harness,
    evm_addr: Address,
    runtime: &[u8],
) -> Result<u64> {
    use fvm_ipld_blockstore::Block;
    use multihash_codetable::Code as MhCode;

    // Resolve EVM actor Wasm code CID from bundle by name.
    let evm_code = bundle_code_by_name(h, "evm")?.expect("evm code in bundle");

    // Put runtime bytecode as a raw IPLD block.
    let bs = h.tester.state_tree.as_ref().unwrap().store();
    let bytecode_blk = Block::new(fvm_ipld_encoding::IPLD_RAW, runtime);
    let bytecode_cid = bs.put(MhCode::Blake2b256, &bytecode_blk)?;

    // Compute keccak256 hash of runtime for bytecode_hash (32 bytes).
    let mut bytecode_hash = [0u8; 32];
    {
        use multihash_codetable::MultihashDigest;
        let mh = multihash_codetable::Code::Keccak256.digest(runtime);
        bytecode_hash.copy_from_slice(mh.digest());
    }

    // Minimal EVM state tuple matching actors/evm State serialization.
    #[derive(fvm_ipld_encoding::tuple::Serialize_tuple)]
    struct EvmState {
        bytecode: Cid,
        #[serde(with = "fvm_ipld_encoding::strict_bytes")]
        bytecode_hash: [u8; 32],
        contract_state: Cid,
        transient_data: Option<()>,
        nonce: u64,
        tombstone: Option<()>,
        delegations: Option<Cid>,
        delegation_nonces: Option<Cid>,
        delegation_storage: Option<Cid>,
    }

    let st = EvmState {
        bytecode: bytecode_cid,
        bytecode_hash,
        contract_state: Cid::default(),
        transient_data: None,
        nonce: 0,
        tombstone: None,
        delegations: None,
        delegation_nonces: None,
        delegation_storage: None,
    };
    let st_cid = bs.put_cbor(&st, Code::Blake2b256)?;

    // Register the address, install the actor with delegated_address set to the f4 address.
    let stree = h.tester.state_tree.as_mut().unwrap();
    let id = stree.register_new_address(&evm_addr).unwrap();
    let act = fvm::state_tree::ActorState::new(evm_code, st_cid, TokenAmount::default(), 0, Some(evm_addr));
    stree.set_actor(id, act);
    Ok(id)
}

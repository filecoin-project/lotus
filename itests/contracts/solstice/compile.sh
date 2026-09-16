#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 /path/to/solstice" >&2
	exit 2
fi

source_dir="$(cd "$1" && pwd -P)"
output_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"

proxy_path="lib/openzeppelin-contracts/contracts/proxy/ERC1967/ERC1967Proxy.sol"

forge build --root "$source_dir" --sizes src/ServiceRewardsActor.sol src/StreamWeightActor.sol "$proxy_path"

commit="$(git -C "$source_dir" rev-parse HEAD)"
if [[ -n "$(git -C "$source_dir" status --porcelain)" ]]; then
	dirty=true
else
	dirty=false
fi

sra_artifact="$source_dir/out/ServiceRewardsActor.sol/ServiceRewardsActor.json"
swa_artifact="$source_dir/out/StreamWeightActor.sol/StreamWeightActor.json"
proxy_artifact="$source_dir/out/ERC1967Proxy.sol/ERC1967Proxy.json"

jq -j '.bytecode.object | sub("^0x"; "")' "$sra_artifact" > "$output_dir/ServiceRewardsActor.hex"
jq -j '.bytecode.object | sub("^0x"; "")' "$swa_artifact" > "$output_dir/StreamWeightActor.hex"
jq -j '.bytecode.object | sub("^0x"; "")' "$proxy_artifact" > "$output_dir/ERC1967Proxy.hex"

jq -n \
	--arg commit "$commit" \
	--argjson dirty "$dirty" \
	--slurpfile sra "$sra_artifact" \
	--slurpfile swa "$swa_artifact" \
	--slurpfile proxy "$proxy_artifact" \
	'
	def contract($artifact; $bytecode): {
		creationBytecode: $bytecode,
		constructorParameterTypes: [
			$artifact.abi[]
			| select(.type == "constructor")
			| .inputs[].type
		],
		functionSelectors: (
			($artifact.methodIdentifiers // {})
			| to_entries
			| sort_by(.key)
			| map(.value = "0x" + .value)
			| from_entries
		)
	};
	{
		source: {
			commit: $commit,
			dirty: $dirty
		},
		contracts: {
			ERC1967Proxy: contract($proxy[0]; "ERC1967Proxy.hex"),
			ServiceRewardsActor: contract($sra[0]; "ServiceRewardsActor.hex"),
			StreamWeightActor: contract($swa[0]; "StreamWeightActor.hex")
		}
	}
	' > "$output_dir/manifest.json"

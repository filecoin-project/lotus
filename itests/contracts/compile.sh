set -eu
set -o pipefail

#use the solc compiler https://docs.soliditylang.org/en/v0.8.17/installing-solidity.html
# to compile all of the .sol files to their corresponding evm binary files stored as .hex
# solc outputs to stdout a format that we just want to grab the last line of and then remove the trailing newline on that line

find .  -maxdepth 1 -name \*.sol -print0 |
	xargs -0 -I{} bash -euc -o pipefail 'solc --bin   {} |tail -n1 | tr -d "\n" > $(echo {} | sed -e s/.sol$/.hex/)'



#for these contracts we have 2 contracts in the same solidity file 
#this command grabs the correct bytecode for us
for filename in Constructor TestApp ValueSender Create2Factory DeployValueTest; do
	echo $filename
	solc --bin $filename.sol | tail -n5|head -n1 | tr -d "\n" > $filename.hex
done

for filename in TransparentUpgradeableProxy ; do
	echo $filename
	solc --bin $filename.sol | tail -n1| tr -d "\n" > $filename.hex
done

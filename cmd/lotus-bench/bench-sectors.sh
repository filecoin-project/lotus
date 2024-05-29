#!/usr/bin/env bash

# This is an example of how a full sector lifecycle can be benchmarked using `lotus-bench`. The
# script generates an unsealed sector, runs PC1, PC2, C1, and C2, and prints the duration of each
# step. The script also prints the proof length and total duration of the lifecycle.
#
# Change `flags` to `--non-interactive` to run NI-PoRep, and switch `sector_size` to the desired
# sector size. The script assumes that the `lotus-bench` binary is in the same directory as the
# script.
#
# Note that for larger sector sizes, /tmp may not have enough space for the full lifecycle.

set -e
set -o pipefail

tmpdir=/tmp

flags=""
# flags="--non-interactive"
sector_size=2KiB
# sector_size=8MiB
# sector_size=512MiB
# sector_size=32GiB
# sector_size=64GiB

unsealed_file=${tmpdir}/unsealed${sector_size}
sealed_file=${tmpdir}/sealed${sector_size}
cache_dir=${tmpdir}/cache${sector_size}
c1_file=${tmpdir}/c1_${sector_size}.json
proof_out=${tmpdir}/proof_${sector_size}.hex
rm -rf $unsealed_file $sealed_file $cache_dir $c1_file

echo "Generating unsealed sector ..."
read -r unsealed_cid unsealed_size <<< $(./lotus-bench simple addpiece --sector-size $sector_size /dev/zero $unsealed_file | tail -1)
if [ $? -ne 0 ]; then exit 1; fi
echo "Unsealed CID: $unsealed_cid"
echo "Unsealed Size: $unsealed_size"

start_total=$(date +%s%3N)

echo "Running PC1 ..."
echo "./lotus-bench simple precommit1 --sector-size $sector_size $flags $unsealed_file $sealed_file $cache_dir $unsealed_cid $unsealed_size"
start_pc1=$(date +%s%3N)
pc1_output=$(./lotus-bench simple precommit1 --sector-size $sector_size $flags $unsealed_file $sealed_file $cache_dir $unsealed_cid $unsealed_size | tail -1)
if [ $? -ne 0 ]; then exit 1; fi
end_pc1=$(date +%s%3N)
pc1_duration=$((end_pc1 - start_pc1))

echo "Running PC2 ..."
echo "./lotus-bench simple precommit2 --sector-size $sector_size $flags $sealed_file $cache_dir $pc1_output"
start_pc2=$(date +%s%3N)
read -r commd commr <<< $(./lotus-bench simple precommit2 --sector-size $sector_size $flags $sealed_file $cache_dir $pc1_output | tail -1 | sed -E 's/[dr]://g')
if [ $? -ne 0 ]; then exit 1; fi
end_pc2=$(date +%s%3N)
pc2_duration=$((end_pc2 - start_pc2))

echo "CommD CID: $commd"
echo "CommR CID: $commr"

echo "Running C1 ..."
echo "./lotus-bench simple commit1 --sector-size $sector_size $flags $sealed_file $cache_dir ${commd} ${commr} $c1_file"
start_c1=$(date +%s%3N)
./lotus-bench simple commit1 --sector-size $sector_size $flags $sealed_file $cache_dir ${commd} ${commr} $c1_file
end_c1=$(date +%s%3N)
c1_duration=$((end_c1 - start_c1))

echo "Running C2 ..."
echo "./lotus-bench simple commit2 $flags $c1_file"
start_c2=$(date +%s%3N)
proof=$(./lotus-bench simple commit2 $flags $c1_file | tail -1 | sed 's/^proof: //')
if [ $? -ne 0 ]; then exit 1; fi
end_c2=$(date +%s%3N)
c2_duration=$((end_c2 - start_c2))

echo $proof > $proof_out
echo "Wrote proof to $proof_out"

# $proof is hex, calculate the length of it in bytes
proof_len=$(echo "scale=0; ${#proof}/2" | bc)
echo "Proof length: $proof_len"

end_total=$(date +%s%3N)
total_duration=$((end_total - start_total))

echo "PC1 duration: $((pc1_duration / 1000)).$((pc1_duration % 1000)) seconds"
echo "PC2 duration: $((pc2_duration / 1000)).$((pc2_duration % 1000)) seconds"
echo "C1 duration: $((c1_duration / 1000)).$((c1_duration % 1000)) seconds"
echo "C2 duration: $((c2_duration / 1000)).$((c2_duration % 1000)) seconds"
echo "Total duration: $((total_duration / 1000)).$((total_duration % 1000)) seconds"
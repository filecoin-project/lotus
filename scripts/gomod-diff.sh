#!/usr/bin/env bash

if [ $# -ne 2 ]; then
    echo "./gomod-diff.sh [refA] [refB]"
    exit 1
fi

temp=$(mktemp -d)
repo=$(pwd)

cd "$temp"
echo "running in $temp"

git clone $repo a
git clone $repo b

cd a
git checkout $1

cd ../b
git checkout $2
make deps
make -j10 buildall

cd ../a
make deps
make -j10 buildall

go mod vendor
cd ../b
go mod vendor

cd ..
diff -Naur --color b/vendor a/vendor
diff -Naur --color b/vendor a/vendor > mod.diff
echo "Saved to $temp/mod.diff"

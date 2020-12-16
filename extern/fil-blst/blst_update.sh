#!/bin/bash

# We need access to the blst c files. At this time we just keep a copy of
# blst in this repo so that it can be build directly using CGO.

rm -fr blst
git clone https://github.com/supranational/blst
cd blst

echo "" >>  ../blst_version.txt
git log HEAD^..HEAD >> ../blst_version.txt

# Make blst a part of this repo
rm -fr .git
git add .
git add -f go.mod

# Build libblst.a
./build.sh

cd ..



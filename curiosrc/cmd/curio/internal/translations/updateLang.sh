#!/bin/bash

#OP: Only run if some file in ../guidedsetup* is newer than catalog.go
# Change this condition if using translations more widely.
if [ "$(find ../../guidedsetup/* -newer catalog.go)" ] || [ "$(find locales/* -newer catalog.go)" ]; then
  gotext -srclang=en update -out=catalog.go -lang=en,zh,ko github.com/filecoin-project/lotus/cmd/curio/guidedsetup
  go run knowns/main.go locales/zh locales/ko
fi

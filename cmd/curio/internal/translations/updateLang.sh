#!/bin/bash

#OP: Only run if some file in ../guidedsetup* is newer than catalog.go
# Change this condition if using translations more widely.
if [ "$(find ../../guidedsetup/* -newermt "$(date -d '1 minute ago')" -newer catalog.go)" ] || [ "$(find locales/* -newermt "$(date -d '1 minute ago')" -newer catalog.go)" ]; then
  gotext -srclang=en update -out=catalog.go -lang=en,zh,ko github.com/filecoin-project/lotus/cmd/curio/guidedsetup
  go run knowns/main.go locales/zh locales/ko
fi

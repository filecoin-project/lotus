SHELL=/usr/bin/env bash

lang: doupdate knowns

doupdate:
	@echo "Updating translations"
	gotext -srclang=en update -out=catalog.go -lang=en,zh,ko github.com/filecoin-project/lotus/cmd/curio/guidedsetup

knowns: 
	go run knowns/main.go locales/zh locales/ko
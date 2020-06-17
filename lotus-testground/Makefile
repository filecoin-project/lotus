all:

deps:
	git submodule update --init
	cd extern/filecoin-ffi && make

run-exec:
	testground run single --plan=network --testcase=lotus-network --builder=exec:go --runner=local:exec --instances=2

run-docker:
	testground run single --plan=network --testcase=lotus-network --builder=docker:go --runner=local:docker --instances=2

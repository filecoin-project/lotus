package api

type Version struct {
	Version string

	// TODO: git commit / os / genesis cid?
}

type API interface {
	// chain

	// // head

	// messages

	// // wait
	// // send
	// // status
	// // mpool
	// // // ls / show / rm

	// dag

	// // get block
	// // (cli: show / info)

	// network

	// // peers
	// // ping
	// // connect

	// client

	// miner

	// // create
	// // owner
	// // power
	// // set-price
	// // set-perrid

	// // UX ?

	// wallet

	// // import
	// // export
	// // list
	// // (on cli - cmd to list associations)

	// dht

	// // need ?

	// paych

	// // todo

	// retrieval

	// // retrieve piece

	// Other

	// // ID (on cli - print with other info)

	Version() Version
}

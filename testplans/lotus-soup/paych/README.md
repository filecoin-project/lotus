# Payment channels end-to-end tests

This package contains the following test cases, each of which is described
further below.

- Payment channels stress test case (`stress.go`).

## Payment channels stress test case (`stress.go`)

***WIP | blocked due to https://github.com/filecoin-project/lotus/issues/2297***

This test case turns all clients into payment receivers and senders.
The first member to start in the group becomes the _receiver_.
All other members become _senders_.

The _senders_ will open a single payment channel to the _receiver_, and will
wait for the message to be posted on-chain. We are setting
`build.MessageConfidence=1`, in order to accelerate the test. So we'll only wait
for a single tipset confirmation once we witness the message.

Once the message is posted, we load the payment channel actor address and create
as many lanes as the `lane_count` test parameter dictates.

When then fetch our total balance, and start sending it on the payment channel,
round-robinning across all lanes, until our balance is extinguished.

**TODO:**

- [ ] Assertions, metrics, etc. Actually gather statistics. Right now this is
  just a smoke test, and it fails.
- [ ] Implement the _receiver_ logic.
- [ ] Model test lifetime by signalling end.
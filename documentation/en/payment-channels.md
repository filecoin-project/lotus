# Payment Channels

Payment channels are used to transfer funds between two actors.

For example in lotus a payment channel is created when a client wants to fetch data from a provider.
The client sends vouchers for the payment channel, and the provider sends data in response.

The payment channel is created on-chain with an initial amount.
Vouchers allow the client and the provider to exchange funds incrementally off-chain.
The provider can submit vouchers to chain at any stage.
Either party to the payment channel can settle the payment channel on chain.
After a settlement period (currently 12 hours) either party to the payment channel can call collect on chain.
Collect sends the value of submitted vouchers to the channel recipient (the provider), and refunds the remaining channel balance to the channel creator (the client).

Vouchers have a lane, a nonce and a value, where vouchers with a higher nonce supersede vouchers with a lower nonce in the same lane.
Each deal is created on a different lane. 

Note that payment channels and vouchers can be used for any situation in which two parties need to incrementally transfer value between each other off-chain.

## Using the CLI

For example a client creates a payment channel to a provider with value 10 FIL.

```sh
$ lotus paych add-funds <client addr> <provider addr> 10
<channel addr>
```

The client creates a voucher in lane 0 (implied) with nonce 1 (implied) and value 2.

```sh
$ lotus paych voucher create <channel addr> 2
<voucher>
```

The client sends the voucher to the provider and the provider adds the voucher to their local store.

```sh
$ lotus paych voucher add <channel addr> <voucher>
```

The provider sends some data to the client.

The client creates a voucher in lane 0 (implied) with nonce 2 (implied) and value 4.

```sh
$ lotus paych voucher create <channel addr> 4
<voucher>
```

The client sends the voucher to the provider and the provider adds the voucher and sends back more data.
etc.

The client can add value to the channel after it has been created by calling `paych add-funds` with the same client and provider addresses.

```sh
$ lotus paych add-funds <client addr> <provider addr> 5
<channel addr> # Same address as above. Channel now has 15
```

Once the client has received all their data, they may settle the channel.
Note that settlement doesn't have to be done immediately.
For example the client may keep the channel open as long as it wants to continue making deals with the provider.

```sh
$ lotus paych settle <channel addr>
```

The provider can submit vouchers to chain (note that lotus does this automatically when it sees a settle message appear on chain).
The provider may have received many vouchers with incrementally higher values.
The provider should submit the best vouchers. Note that there will be one best voucher for each lane.

```sh
$ lotus paych voucher best-spendable <channel addr>
<voucher>
<voucher>
<voucher>

$ lotus paych voucher submit <channel addr> <voucher>
```

Once the settlement period is over, either the client or provider can call collect to disburse funds.

```sh
$ lotus paych collect <channel addr>
```

Check the status of a channel that is still being created using `lotus paych status-by-from-to`.

```sh
$ lotus paych status-by-from-to <from addr> <to addr>
Creating channel
  From:          t3sb6xzvs6rhlziatagevxpp3dwapdolurtkpn4kyh3kgoo4tn5o7lutjqlsnvpceztlhxu3lzzfe34rvpsjgq
  To:            t1zip4sblhyrn4oxygzsm6nafbsynp2avmk3xafea
  Pending Amt:   10000
  Wait Sentinel: bafy2bzacedk2jidsyxcynusted35t5ipkhu2kpiodtwyjr3pimrhke6f5pqbm
```

Check the status of a channel that has been created using `lotus paych status`.

```sh
$ lotus paych status <channel addr>
Channel exists
  Channel:              t2nydpzhmeqkmid5smtqnowlr2mr5az6rexpmyv6i
  From:                 t3sb6xzvs6rhlziatagevxpp3dwapdolurtkpn4kyh3kgoo4tn5o7lutjqlsnvpceztlhxu3lzzfe34rvpsjgq
  To:                   t1zip4sblhyrn4oxygzsm6nafbsynp2avmk3xafea
  Confirmed Amt:        10000
  Pending Amt:          6000
  Queued Amt:           3000
  Voucher Redeemed Amt: 2000
```

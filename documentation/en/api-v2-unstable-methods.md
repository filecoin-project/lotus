# Groups
* [Chain](#Chain)
  * [ChainGetTipSet](#ChainGetTipSet)
## Chain
The Chain method group contains methods for interacting with
the blockchain.


### ChainGetTipSet
ChainGetTipSet retrieves a tipset that corresponds to the specified selector
criteria. The criteria can be provided in the form of a tipset key, a
blockchain height including an optional fallback to previous non-null tipset,
or a designated tag such as "latest" or "finalized".

The "Finalized" tag returns the tipset that is considered finalized based on
the consensus protocol of the current node, either Filecoin EC Finality or
Filecoin Fast Finality (F3). The finalized tipset selection gracefully falls
back to EC finality in cases where F3 isn't ready or not running.

In a case where no selector is provided, the method defaults to return the
tipset tagged with "latest".

For more details, refer to the types.TipSetSelector and
types.NewTipSetSelector.

Example usage:

	selector := types.TipSetSelectors.Latest
	tipSet, err := node.ChainGetTipSet(context.Background(), selector)
	if err != nil {
		fmt.Println("Error retrieving tipset:", err)
		return
	}
	fmt.Printf("Latest TipSet: %v\n", tipSet)


Perms: read

Inputs:
```json
[
  {
    "tag": "finalized"
  }
]
```

Response:
```json
{
  "Cids": null,
  "Blocks": null,
  "Height": 0
}
```


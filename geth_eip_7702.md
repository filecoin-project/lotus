commit f808d7357ed4076b224a8c6fe47893ce022f9409
Author: Matt Garnett <lightclient@protonmail.com>
Date:   Mon Dec 16 03:29:37 2024 -0700

    all: implement eip-7702 set code tx (#30078)
    
    This PR implements EIP-7702: "Set EOA account code".
    Specification: https://eips.ethereum.org/EIPS/eip-7702
    
    > Add a new transaction type that adds a list of `[chain_id, address,
    nonce, y_parity, r, s]` authorization tuples. For each tuple, write a
    delegation designator `(0xef0100 ++ address)` to the signing account’s
    code. All code reading operations must load the code pointed to by the
    designator.
    
    ---------
    
    Co-authored-by: Mario Vega <marioevz@gmail.com>
    Co-authored-by: Martin Holst Swende <martin@swende.se>
    Co-authored-by: Felix Lange <fjl@twurst.com>

diff --git a/accounts/external/backend.go b/accounts/external/backend.go
index 62322753d..42eaf661c 100644
--- a/accounts/external/backend.go
+++ b/accounts/external/backend.go
@@ -215,7 +215,7 @@ func (api *ExternalSigner) SignTx(account accounts.Account, tx *types.Transactio
 	switch tx.Type() {
 	case types.LegacyTxType, types.AccessListTxType:
 		args.GasPrice = (*hexutil.Big)(tx.GasPrice())
-	case types.DynamicFeeTxType, types.BlobTxType:
+	case types.DynamicFeeTxType, types.BlobTxType, types.SetCodeTxType:
 		args.MaxFeePerGas = (*hexutil.Big)(tx.GasFeeCap())
 		args.MaxPriorityFeePerGas = (*hexutil.Big)(tx.GasTipCap())
 	default:
diff --git a/cmd/evm/eofparse.go b/cmd/evm/eofparse.go
index df8581146..92182a53b 100644
--- a/cmd/evm/eofparse.go
+++ b/cmd/evm/eofparse.go
@@ -36,7 +36,7 @@ var jt vm.JumpTable
 const initcode = "INITCODE"
 
 func init() {
-	jt = vm.NewPragueEOFInstructionSetForTesting()
+	jt = vm.NewEOFInstructionSetForTesting()
 }
 
 var (
diff --git a/cmd/evm/eofparse_test.go b/cmd/evm/eofparse_test.go
index 9b17039f5..cda4b38fc 100644
--- a/cmd/evm/eofparse_test.go
+++ b/cmd/evm/eofparse_test.go
@@ -43,7 +43,7 @@ func FuzzEofParsing(f *testing.F) {
 	// And do the fuzzing
 	f.Fuzz(func(t *testing.T, data []byte) {
 		var (
-			jt = vm.NewPragueEOFInstructionSetForTesting()
+			jt = vm.NewEOFInstructionSetForTesting()
 			c  vm.Container
 		)
 		cpy := common.CopyBytes(data)
diff --git a/cmd/evm/internal/t8ntool/execution.go b/cmd/evm/internal/t8ntool/execution.go
index 7a0de86a1..aef497885 100644
--- a/cmd/evm/internal/t8ntool/execution.go
+++ b/cmd/evm/internal/t8ntool/execution.go
@@ -70,11 +70,11 @@ type ExecutionResult struct {
 	CurrentExcessBlobGas *math.HexOrDecimal64  `json:"currentExcessBlobGas,omitempty"`
 	CurrentBlobGasUsed   *math.HexOrDecimal64  `json:"blobGasUsed,omitempty"`
 	RequestsHash         *common.Hash          `json:"requestsHash,omitempty"`
-	Requests             [][]byte              `json:"requests,omitempty"`
+	Requests             [][]byte              `json:"requests"`
 }
 
 type executionResultMarshaling struct {
-	Requests []hexutil.Bytes `json:"requests,omitempty"`
+	Requests []hexutil.Bytes `json:"requests"`
 }
 
 type ommer struct {
diff --git a/cmd/evm/internal/t8ntool/gen_execresult.go b/cmd/evm/internal/t8ntool/gen_execresult.go
index 0da94f5ca..38310b9f2 100644
--- a/cmd/evm/internal/t8ntool/gen_execresult.go
+++ b/cmd/evm/internal/t8ntool/gen_execresult.go
@@ -31,7 +31,7 @@ func (e ExecutionResult) MarshalJSON() ([]byte, error) {
 		CurrentExcessBlobGas *math.HexOrDecimal64  `json:"currentExcessBlobGas,omitempty"`
 		CurrentBlobGasUsed   *math.HexOrDecimal64  `json:"blobGasUsed,omitempty"`
 		RequestsHash         *common.Hash          `json:"requestsHash,omitempty"`
-		Requests             []hexutil.Bytes       `json:"requests,omitempty"`
+		Requests             []hexutil.Bytes       `json:"requests"`
 	}
 	var enc ExecutionResult
 	enc.StateRoot = e.StateRoot
@@ -74,7 +74,7 @@ func (e *ExecutionResult) UnmarshalJSON(input []byte) error {
 		CurrentExcessBlobGas *math.HexOrDecimal64  `json:"currentExcessBlobGas,omitempty"`
 		CurrentBlobGasUsed   *math.HexOrDecimal64  `json:"blobGasUsed,omitempty"`
 		RequestsHash         *common.Hash          `json:"requestsHash,omitempty"`
-		Requests             []hexutil.Bytes       `json:"requests,omitempty"`
+		Requests             []hexutil.Bytes       `json:"requests"`
 	}
 	var dec ExecutionResult
 	if err := json.Unmarshal(input, &dec); err != nil {
diff --git a/cmd/evm/internal/t8ntool/transaction.go b/cmd/evm/internal/t8ntool/transaction.go
index 7f66ba4d8..64e21b24f 100644
--- a/cmd/evm/internal/t8ntool/transaction.go
+++ b/cmd/evm/internal/t8ntool/transaction.go
@@ -133,7 +133,7 @@ func Transaction(ctx *cli.Context) error {
 			r.Address = sender
 		}
 		// Check intrinsic gas
-		if gas, err := core.IntrinsicGas(tx.Data(), tx.AccessList(), tx.To() == nil,
+		if gas, err := core.IntrinsicGas(tx.Data(), tx.AccessList(), tx.AuthList(), tx.To() == nil,
 			chainConfig.IsHomestead(new(big.Int)), chainConfig.IsIstanbul(new(big.Int)), chainConfig.IsShanghai(new(big.Int), 0)); err != nil {
 			r.Error = err
 			results = append(results, r)
diff --git a/cmd/evm/t8n_test.go b/cmd/evm/t8n_test.go
index 85caf010b..27c6c4316 100644
--- a/cmd/evm/t8n_test.go
+++ b/cmd/evm/t8n_test.go
@@ -287,6 +287,14 @@ func TestT8n(t *testing.T) {
 			output: t8nOutput{alloc: true, result: true},
 			expOut: "exp.json",
 		},
+		{ // Prague test, EIP-7702 transaction
+			base: "./testdata/33",
+			input: t8nInput{
+				"alloc.json", "txs.json", "env.json", "Prague", "",
+			},
+			output: t8nOutput{alloc: true, result: true},
+			expOut: "exp.json",
+		},
 	} {
 		args := []string{"t8n"}
 		args = append(args, tc.output.get()...)
diff --git a/cmd/evm/testdata/1/exp.json b/cmd/evm/testdata/1/exp.json
index d1351e5b7..50662f35e 100644
--- a/cmd/evm/testdata/1/exp.json
+++ b/cmd/evm/testdata/1/exp.json
@@ -40,6 +40,7 @@
       }
     ],
     "currentDifficulty": "0x20000",
-    "gasUsed": "0x5208"
+    "gasUsed": "0x5208",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/13/exp2.json b/cmd/evm/testdata/13/exp2.json
index babce3592..6415a4f1f 100644
--- a/cmd/evm/testdata/13/exp2.json
+++ b/cmd/evm/testdata/13/exp2.json
@@ -37,6 +37,7 @@
     ],
     "currentDifficulty": "0x20000",
     "gasUsed": "0x109a0",
-    "currentBaseFee": "0x36b"
+    "currentBaseFee": "0x36b",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/14/exp.json b/cmd/evm/testdata/14/exp.json
index 26d49173c..300217ee3 100644
--- a/cmd/evm/testdata/14/exp.json
+++ b/cmd/evm/testdata/14/exp.json
@@ -8,6 +8,7 @@
     "currentDifficulty": "0x2000020000000",
     "receipts": [],
     "gasUsed": "0x0",
-    "currentBaseFee": "0x500"
+    "currentBaseFee": "0x500",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/14/exp2.json b/cmd/evm/testdata/14/exp2.json
index cd75b47d5..6d139e371 100644
--- a/cmd/evm/testdata/14/exp2.json
+++ b/cmd/evm/testdata/14/exp2.json
@@ -8,6 +8,7 @@
     "receipts": [],
     "currentDifficulty": "0x1ff8020000000",
     "gasUsed": "0x0",
-    "currentBaseFee": "0x500"
+    "currentBaseFee": "0x500",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/14/exp_berlin.json b/cmd/evm/testdata/14/exp_berlin.json
index 5c00ef130..c952d0f51 100644
--- a/cmd/evm/testdata/14/exp_berlin.json
+++ b/cmd/evm/testdata/14/exp_berlin.json
@@ -8,6 +8,7 @@
     "receipts": [],
     "currentDifficulty": "0x1ff9000000000",
     "gasUsed": "0x0",
-    "currentBaseFee": "0x500"
+    "currentBaseFee": "0x500",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/19/exp_arrowglacier.json b/cmd/evm/testdata/19/exp_arrowglacier.json
index dd49f7d02..0822fcc29 100644
--- a/cmd/evm/testdata/19/exp_arrowglacier.json
+++ b/cmd/evm/testdata/19/exp_arrowglacier.json
@@ -8,6 +8,7 @@
     "currentDifficulty": "0x2000000200000",
     "receipts": [],
     "gasUsed": "0x0",
-    "currentBaseFee": "0x500"
+    "currentBaseFee": "0x500",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/19/exp_grayglacier.json b/cmd/evm/testdata/19/exp_grayglacier.json
index 86fd8e6c1..e80c9eb00 100644
--- a/cmd/evm/testdata/19/exp_grayglacier.json
+++ b/cmd/evm/testdata/19/exp_grayglacier.json
@@ -1,13 +1,14 @@
 {
-    "result": {
-      "stateRoot": "0x6f058887ca01549716789c380ede95aecc510e6d1fdc4dbf67d053c7c07f4bdc",
-      "txRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
-      "receiptsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
-      "logsHash": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
-      "logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
-      "receipts": [],
-      "currentDifficulty": "0x2000000004000",
-      "gasUsed": "0x0",
-      "currentBaseFee": "0x500"
-    }
-}
\ No newline at end of file
+  "result": {
+    "stateRoot": "0x6f058887ca01549716789c380ede95aecc510e6d1fdc4dbf67d053c7c07f4bdc",
+    "txRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
+    "receiptsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
+    "logsHash": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
+    "logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
+    "receipts": [],
+    "currentDifficulty": "0x2000000004000",
+    "gasUsed": "0x0",
+    "currentBaseFee": "0x500",
+    "requests": null
+  }
+}
diff --git a/cmd/evm/testdata/19/exp_london.json b/cmd/evm/testdata/19/exp_london.json
index 9e9a17da9..a17f68448 100644
--- a/cmd/evm/testdata/19/exp_london.json
+++ b/cmd/evm/testdata/19/exp_london.json
@@ -8,6 +8,7 @@
     "currentDifficulty": "0x2000080000000",
     "receipts": [],
     "gasUsed": "0x0",
-    "currentBaseFee": "0x500"
+    "currentBaseFee": "0x500",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/23/exp.json b/cmd/evm/testdata/23/exp.json
index 22dde0a27..7f36165e3 100644
--- a/cmd/evm/testdata/23/exp.json
+++ b/cmd/evm/testdata/23/exp.json
@@ -21,6 +21,7 @@
       }
     ],
     "currentDifficulty": "0x20000",
-    "gasUsed": "0x520b"
+    "gasUsed": "0x520b",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/24/exp.json b/cmd/evm/testdata/24/exp.json
index ac571d149..8f380c662 100644
--- a/cmd/evm/testdata/24/exp.json
+++ b/cmd/evm/testdata/24/exp.json
@@ -51,6 +51,7 @@
     ],
     "currentDifficulty": null,
     "gasUsed": "0x10306",
-    "currentBaseFee": "0x500"
+    "currentBaseFee": "0x500",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/25/exp.json b/cmd/evm/testdata/25/exp.json
index 1cb521794..a67463376 100644
--- a/cmd/evm/testdata/25/exp.json
+++ b/cmd/evm/testdata/25/exp.json
@@ -34,6 +34,7 @@
     ],
     "currentDifficulty": null,
     "gasUsed": "0x5208",
-    "currentBaseFee": "0x460"
+    "currentBaseFee": "0x460",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/26/exp.json b/cmd/evm/testdata/26/exp.json
index 4815e5cb6..d6275270f 100644
--- a/cmd/evm/testdata/26/exp.json
+++ b/cmd/evm/testdata/26/exp.json
@@ -15,6 +15,7 @@
     "currentDifficulty": null,
     "gasUsed": "0x0",
     "currentBaseFee": "0x500",
-    "withdrawalsRoot": "0x4921c0162c359755b2ae714a0978a1dad2eb8edce7ff9b38b9b6fc4cbc547eb5"
+    "withdrawalsRoot": "0x4921c0162c359755b2ae714a0978a1dad2eb8edce7ff9b38b9b6fc4cbc547eb5",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/28/exp.json b/cmd/evm/testdata/28/exp.json
index 75c715e97..b86c2d8de 100644
--- a/cmd/evm/testdata/28/exp.json
+++ b/cmd/evm/testdata/28/exp.json
@@ -42,6 +42,7 @@
     "currentBaseFee": "0x9",
     "withdrawalsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
     "currentExcessBlobGas": "0x0",
-    "blobGasUsed": "0x20000"
+    "blobGasUsed": "0x20000",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/29/exp.json b/cmd/evm/testdata/29/exp.json
index c4c001ec1..7fbdc1828 100644
--- a/cmd/evm/testdata/29/exp.json
+++ b/cmd/evm/testdata/29/exp.json
@@ -40,6 +40,7 @@
     "currentBaseFee": "0x9",
     "withdrawalsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
     "currentExcessBlobGas": "0x0",
-    "blobGasUsed": "0x0"
+    "blobGasUsed": "0x0",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/3/exp.json b/cmd/evm/testdata/3/exp.json
index 7230dca2c..831c07859 100644
--- a/cmd/evm/testdata/3/exp.json
+++ b/cmd/evm/testdata/3/exp.json
@@ -34,6 +34,7 @@
       }
     ],
     "currentDifficulty": "0x20000",
-    "gasUsed": "0x521f"
+    "gasUsed": "0x521f",
+    "requests": null
   }
 }
diff --git a/cmd/evm/testdata/30/exp.json b/cmd/evm/testdata/30/exp.json
index f0b19c6b3..a206c3bbd 100644
--- a/cmd/evm/testdata/30/exp.json
+++ b/cmd/evm/testdata/30/exp.json
@@ -59,6 +59,7 @@
     "currentBaseFee": "0x7",
     "withdrawalsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
     "currentExcessBlobGas": "0x0",
-    "blobGasUsed": "0x0"
+    "blobGasUsed": "0x0",
+    "requests": null
   }
-}
\ No newline at end of file
+}
diff --git a/cmd/evm/testdata/33/README.md b/cmd/evm/testdata/33/README.md
new file mode 100644
index 000000000..24bea566e
--- /dev/null
+++ b/cmd/evm/testdata/33/README.md
@@ -0,0 +1 @@
+This test sets some EIP-7702 delegations and calls them.
diff --git a/cmd/evm/testdata/33/alloc.json b/cmd/evm/testdata/33/alloc.json
new file mode 100644
index 000000000..6874a6b33
--- /dev/null
+++ b/cmd/evm/testdata/33/alloc.json
@@ -0,0 +1,30 @@
+{
+  "0x8a0a19589531694250d570040a0c4b74576919b8": {
+    "nonce": "0x00",
+    "balance": "0x0de0b6b3a7640000",
+    "code": "0x600060006000600060007310000000000000000000000000000000000000015af1600155600060006000600060007310000000000000000000000000000000000000025af16002553d600060003e600051600355",
+    "storage": {
+      "0x01": "0x0100",
+      "0x02": "0x0100",
+      "0x03": "0x0100"
+    }
+  },
+  "0x000000000000000000000000000000000000aaaa": {
+    "nonce": "0x00",
+    "balance": "0x4563918244f40000",
+    "code": "0x58808080600173703c4b2bd70c169f5717101caee543299fc946c75af100",
+    "storage": {}
+  },
+  "0x000000000000000000000000000000000000bbbb": {
+    "nonce": "0x00",
+    "balance": "0x29a2241af62c0000",
+    "code": "0x6042805500",
+    "storage": {}
+  },
+  "0x71562b71999873DB5b286dF957af199Ec94617F7": {
+    "nonce": "0x00",
+    "balance": "0x6124fee993bc0000",
+    "code": "0x",
+    "storage": {}
+  }
+}
diff --git a/cmd/evm/testdata/33/env.json b/cmd/evm/testdata/33/env.json
new file mode 100644
index 000000000..70bb7f981
--- /dev/null
+++ b/cmd/evm/testdata/33/env.json
@@ -0,0 +1,14 @@
+{
+  "currentCoinbase": "0x2adc25665018aa1fe0e6bc666dac8fc2697ff9ba",
+  "currentGasLimit": "71794957647893862",
+  "currentNumber": "1",
+  "currentTimestamp": "1000",
+  "currentRandom": "0",
+  "currentDifficulty": "0",
+  "blockHashes": {},
+  "ommers": [],
+  "currentBaseFee": "7",
+  "parentUncleHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
+  "withdrawals": [],
+  "parentBeaconBlockRoot": "0x0000000000000000000000000000000000000000000000000000000000000000"
+}
diff --git a/cmd/evm/testdata/33/exp.json b/cmd/evm/testdata/33/exp.json
new file mode 100644
index 000000000..ae82ef3ef
--- /dev/null
+++ b/cmd/evm/testdata/33/exp.json
@@ -0,0 +1,62 @@
+{
+  "alloc": {
+    "0x000000000000000000000000000000000000aaaa": {
+      "code": "0x58808080600173703c4b2bd70c169f5717101caee543299fc946c75af100",
+      "balance": "0x4563918244f40000"
+    },
+    "0x000000000000000000000000000000000000bbbb": {
+      "code": "0x6042805500",
+      "balance": "0x29a2241af62c0000"
+    },
+    "0x2adc25665018aa1fe0e6bc666dac8fc2697ff9ba": {
+      "balance": "0x2bf52"
+    },
+    "0x703c4b2bd70c169f5717101caee543299fc946c7": {
+      "code": "0xef0100000000000000000000000000000000000000bbbb",
+      "storage": {
+        "0x0000000000000000000000000000000000000000000000000000000000000042": "0x0000000000000000000000000000000000000000000000000000000000000042"
+      },
+      "balance": "0x1",
+      "nonce": "0x1"
+    },
+    "0x71562b71999873db5b286df957af199ec94617f7": {
+      "code": "0xef0100000000000000000000000000000000000000aaaa",
+      "balance": "0x6124fee993afa30e",
+      "nonce": "0x2"
+    },
+    "0x8a0a19589531694250d570040a0c4b74576919b8": {
+      "code": "0x600060006000600060007310000000000000000000000000000000000000015af1600155600060006000600060007310000000000000000000000000000000000000025af16002553d600060003e600051600355",
+      "storage": {
+        "0x0000000000000000000000000000000000000000000000000000000000000001": "0x0000000000000000000000000000000000000000000000000000000000000100",
+        "0x0000000000000000000000000000000000000000000000000000000000000002": "0x0000000000000000000000000000000000000000000000000000000000000100",
+        "0x0000000000000000000000000000000000000000000000000000000000000003": "0x0000000000000000000000000000000000000000000000000000000000000100"
+      },
+      "balance": "0xde0b6b3a7640000"
+    }
+  },
+  "result": {
+    "stateRoot": "0x9fdcacd4510e93c4488e537dc51578b5c6d505771db64a2610036eeb4be7b26f",
+    "txRoot": "0x5d13a0b074e80388dc754da92b22922313a63417b3e25a10f324935e09697a53",
+    "receiptsRoot": "0x504c5d86c34391f70d210e6c482615b391db4bdb9f43479366399d9c5599850a",
+    "logsHash": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
+    "logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000","receipts": [{
+      "type": "0x4",
+      "root": "0x",
+      "status": "0x1",
+      "cumulativeGasUsed": "0x15fa9",
+      "logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000","logs": null,"transactionHash": "0x0417aab7c1d8a3989190c3167c132876ce9b8afd99262c5a0f9d06802de3d7ef",
+      "contractAddress": "0x0000000000000000000000000000000000000000",
+      "gasUsed": "0x15fa9",
+      "effectiveGasPrice": null,
+      "blockHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
+      "transactionIndex": "0x0"
+    }
+  ],
+  "currentDifficulty": null,
+  "gasUsed": "0x15fa9",
+  "currentBaseFee": "0x7",
+  "withdrawalsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
+  "requestsHash": "0xe3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
+  "requests": []
+}
+}
diff --git a/cmd/evm/testdata/33/txs.json b/cmd/evm/testdata/33/txs.json
new file mode 100644
index 000000000..9c4db138c
--- /dev/null
+++ b/cmd/evm/testdata/33/txs.json
@@ -0,0 +1,37 @@
+[
+  {
+    "type": "0x4",
+    "chainId": "0x1",
+    "nonce": "0x0",
+    "to": "0x71562b71999873db5b286df957af199ec94617f7",
+    "gas": "0x7a120",
+    "gasPrice": null,
+    "maxPriorityFeePerGas": "0x2",
+    "maxFeePerGas": "0x12a05f200",
+    "value": "0x0",
+    "input": "0x",
+    "accessList": [],
+    "authorizationList": [
+      {
+        "chainId": "0x1",
+        "address": "0x000000000000000000000000000000000000aaaa",
+        "nonce": "0x1",
+        "v": "0x1",
+        "r": "0xf7e3e597fc097e71ed6c26b14b25e5395bc8510d58b9136af439e12715f2d721",
+        "s": "0x6cf7c3d7939bfdb784373effc0ebb0bd7549691a513f395e3cdabf8602724987"
+      },
+      {
+        "chainId": "0x0",
+        "address": "0x000000000000000000000000000000000000bbbb",
+        "nonce": "0x0",
+        "v": "0x1",
+        "r": "0x5011890f198f0356a887b0779bde5afa1ed04e6acb1e3f37f8f18c7b6f521b98",
+        "s": "0x56c3fa3456b103f3ef4a0acb4b647b9cab9ec4bc68fbcdf1e10b49fb2bcbcf61"
+      }
+    ],
+    "secretKey": "0xb71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291",
+    "v": "0x0",
+    "r": "0x0",
+    "s": "0x0"
+  }
+]
diff --git a/cmd/evm/testdata/5/exp.json b/cmd/evm/testdata/5/exp.json
index 7d715672c..00af8b084 100644
--- a/cmd/evm/testdata/5/exp.json
+++ b/cmd/evm/testdata/5/exp.json
@@ -18,6 +18,7 @@
     "logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
     "receipts": [],
     "currentDifficulty": "0x20000",
-    "gasUsed": "0x0"
+    "gasUsed": "0x0",
+    "requests": null
   }
 }
diff --git a/core/bench_test.go b/core/bench_test.go
index 6d518e8d3..d37683031 100644
--- a/core/bench_test.go
+++ b/core/bench_test.go
@@ -90,7 +90,7 @@ func genValueTx(nbytes int) func(int, *BlockGen) {
 	data := make([]byte, nbytes)
 	return func(i int, gen *BlockGen) {
 		toaddr := common.Address{}
-		gas, _ := IntrinsicGas(data, nil, false, false, false, false)
+		gas, _ := IntrinsicGas(data, nil, nil, false, false, false, false)
 		signer := gen.Signer()
 		gasPrice := big.NewInt(0)
 		if gen.header.BaseFee != nil {
diff --git a/core/blockchain_test.go b/core/blockchain_test.go
index dc391bb52..a54a90776 100644
--- a/core/blockchain_test.go
+++ b/core/blockchain_test.go
@@ -17,6 +17,7 @@
 package core
 
 import (
+	"bytes"
 	"errors"
 	"fmt"
 	gomath "math"
@@ -36,6 +37,7 @@ import (
 	"github.com/ethereum/go-ethereum/core/state"
 	"github.com/ethereum/go-ethereum/core/types"
 	"github.com/ethereum/go-ethereum/core/vm"
+	"github.com/ethereum/go-ethereum/core/vm/program"
 	"github.com/ethereum/go-ethereum/crypto"
 	"github.com/ethereum/go-ethereum/eth/tracers/logger"
 	"github.com/ethereum/go-ethereum/ethdb"
@@ -4232,3 +4234,95 @@ func TestPragueRequests(t *testing.T) {
 		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
 	}
 }
+
+// TestEIP7702 deploys two delegation designations and calls them. It writes one
+// value to storage which is verified after.
+func TestEIP7702(t *testing.T) {
+	var (
+		config  = *params.MergedTestChainConfig
+		signer  = types.LatestSigner(&config)
+		engine  = beacon.NewFaker()
+		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
+		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
+		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
+		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
+		aa      = common.HexToAddress("0x000000000000000000000000000000000000aaaa")
+		bb      = common.HexToAddress("0x000000000000000000000000000000000000bbbb")
+		funds   = new(big.Int).Mul(common.Big1, big.NewInt(params.Ether))
+	)
+	gspec := &Genesis{
+		Config: &config,
+		Alloc: types.GenesisAlloc{
+			addr1: {Balance: funds},
+			addr2: {Balance: funds},
+			aa: { // The address 0xAAAA calls into addr2
+				Code:    program.New().Call(nil, addr2, 1, 0, 0, 0, 0).Bytes(),
+				Nonce:   0,
+				Balance: big.NewInt(0),
+			},
+			bb: { // The address 0xBBBB sstores 42 into slot 42.
+				Code:    program.New().Sstore(0x42, 0x42).Bytes(),
+				Nonce:   0,
+				Balance: big.NewInt(0),
+			},
+		},
+	}
+
+	// Sign authorization tuples.
+	// The way the auths are combined, it becomes
+	// 1. tx -> addr1 which is delegated to 0xaaaa
+	// 2. addr1:0xaaaa calls into addr2:0xbbbb
+	// 3. addr2:0xbbbb  writes to storage
+	auth1, _ := types.SignAuth(types.Authorization{
+		ChainID: gspec.Config.ChainID.Uint64(),
+		Address: aa,
+		Nonce:   1,
+	}, key1)
+	auth2, _ := types.SignAuth(types.Authorization{
+		ChainID: 0,
+		Address: bb,
+		Nonce:   0,
+	}, key2)
+
+	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 1, func(i int, b *BlockGen) {
+		b.SetCoinbase(aa)
+		txdata := &types.SetCodeTx{
+			ChainID:   gspec.Config.ChainID.Uint64(),
+			Nonce:     0,
+			To:        addr1,
+			Gas:       500000,
+			GasFeeCap: uint256.MustFromBig(newGwei(5)),
+			GasTipCap: uint256.NewInt(2),
+			AuthList:  []types.Authorization{auth1, auth2},
+		}
+		tx := types.MustSignNewTx(key1, signer, txdata)
+		b.AddTx(tx)
+	})
+	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), nil, gspec, nil, engine, vm.Config{}, nil)
+	if err != nil {
+		t.Fatalf("failed to create tester chain: %v", err)
+	}
+	defer chain.Stop()
+	if n, err := chain.InsertChain(blocks); err != nil {
+		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
+	}
+
+	// Verify delegation designations were deployed.
+	state, _ := chain.State()
+	code, want := state.GetCode(addr1), types.AddressToDelegation(auth1.Address)
+	if !bytes.Equal(code, want) {
+		t.Fatalf("addr1 code incorrect: got %s, want %s", common.Bytes2Hex(code), common.Bytes2Hex(want))
+	}
+	code, want = state.GetCode(addr2), types.AddressToDelegation(auth2.Address)
+	if !bytes.Equal(code, want) {
+		t.Fatalf("addr2 code incorrect: got %s, want %s", common.Bytes2Hex(code), common.Bytes2Hex(want))
+	}
+	// Verify delegation executed the correct code.
+	var (
+		fortyTwo = common.BytesToHash([]byte{0x42})
+		actual   = state.GetState(addr2, fortyTwo)
+	)
+	if actual.Cmp(fortyTwo) != 0 {
+		t.Fatalf("addr2 storage wrong: expected %d, got %d", fortyTwo, actual)
+	}
+}
diff --git a/core/error.go b/core/error.go
index 161538fe4..82f7ddcf5 100644
--- a/core/error.go
+++ b/core/error.go
@@ -103,6 +103,8 @@ var (
 	// ErrSenderNoEOA is returned if the sender of a transaction is a contract.
 	ErrSenderNoEOA = errors.New("sender not an eoa")
 
+	// -- EIP-4844 errors --
+
 	// ErrBlobFeeCapTooLow is returned if the transaction fee cap is less than the
 	// blob gas fee of the block.
 	ErrBlobFeeCapTooLow = errors.New("max fee per blob gas less than block blob gas fee")
@@ -112,4 +114,20 @@ var (
 
 	// ErrBlobTxCreate is returned if a blob transaction has no explicit to field.
 	ErrBlobTxCreate = errors.New("blob transaction of type create")
+
+	// -- EIP-7702 errors --
+
+	// Message validation errors:
+	ErrEmptyAuthList   = errors.New("EIP-7702 transaction with empty auth list")
+	ErrSetCodeTxCreate = errors.New("EIP-7702 transaction cannot be used to create contract")
+)
+
+// EIP-7702 state transition errors.
+// Note these are just informational, and do not cause tx execution abort.
+var (
+	ErrAuthorizationWrongChainID       = errors.New("EIP-7702 authorization chain ID mismatch")
+	ErrAuthorizationNonceOverflow      = errors.New("EIP-7702 authorization nonce > 64 bit")
+	ErrAuthorizationInvalidSignature   = errors.New("EIP-7702 authorization has invalid signature")
+	ErrAuthorizationDestinationHasCode = errors.New("EIP-7702 authorization destination is a contract")
+	ErrAuthorizationNonceMismatch      = errors.New("EIP-7702 authorization nonce does not match current account nonce")
 )
diff --git a/core/state/journal.go b/core/state/journal.go
index a2fea6b6e..13332dbd7 100644
--- a/core/state/journal.go
+++ b/core/state/journal.go
@@ -23,7 +23,7 @@ import (
 	"sort"
 
 	"github.com/ethereum/go-ethereum/common"
-	"github.com/ethereum/go-ethereum/core/types"
+	"github.com/ethereum/go-ethereum/crypto"
 	"github.com/holiman/uint256"
 )
 
@@ -192,8 +192,11 @@ func (j *journal) balanceChange(addr common.Address, previous *uint256.Int) {
 	})
 }
 
-func (j *journal) setCode(address common.Address) {
-	j.append(codeChange{account: address})
+func (j *journal) setCode(address common.Address, prevCode []byte) {
+	j.append(codeChange{
+		account:  address,
+		prevCode: prevCode,
+	})
 }
 
 func (j *journal) nonceChange(address common.Address, prev uint64) {
@@ -256,7 +259,8 @@ type (
 		origvalue common.Hash
 	}
 	codeChange struct {
-		account common.Address
+		account  common.Address
+		prevCode []byte
 	}
 
 	// Changes to other state values.
@@ -377,7 +381,7 @@ func (ch nonceChange) copy() journalEntry {
 }
 
 func (ch codeChange) revert(s *StateDB) {
-	s.getStateObject(ch.account).setCode(types.EmptyCodeHash, nil)
+	s.getStateObject(ch.account).setCode(crypto.Keccak256Hash(ch.prevCode), ch.prevCode)
 }
 
 func (ch codeChange) dirtied() *common.Address {
@@ -385,7 +389,10 @@ func (ch codeChange) dirtied() *common.Address {
 }
 
 func (ch codeChange) copy() journalEntry {
-	return codeChange{account: ch.account}
+	return codeChange{
+		account:  ch.account,
+		prevCode: ch.prevCode,
+	}
 }
 
 func (ch storageChange) revert(s *StateDB) {
diff --git a/core/state/state_object.go b/core/state/state_object.go
index 2d542e500..76a3aba92 100644
--- a/core/state/state_object.go
+++ b/core/state/state_object.go
@@ -20,6 +20,7 @@ import (
 	"bytes"
 	"fmt"
 	"maps"
+	"slices"
 	"time"
 
 	"github.com/ethereum/go-ethereum/common"
@@ -541,9 +542,11 @@ func (s *stateObject) CodeSize() int {
 	return size
 }
 
-func (s *stateObject) SetCode(codeHash common.Hash, code []byte) {
-	s.db.journal.setCode(s.address)
+func (s *stateObject) SetCode(codeHash common.Hash, code []byte) (prev []byte) {
+	prev = slices.Clone(s.code)
+	s.db.journal.setCode(s.address, prev)
 	s.setCode(codeHash, code)
+	return prev
 }
 
 func (s *stateObject) setCode(codeHash common.Hash, code []byte) {
diff --git a/core/state/statedb.go b/core/state/statedb.go
index b0603db7f..d279ccfdf 100644
--- a/core/state/statedb.go
+++ b/core/state/statedb.go
@@ -439,11 +439,12 @@ func (s *StateDB) SetNonce(addr common.Address, nonce uint64) {
 	}
 }
 
-func (s *StateDB) SetCode(addr common.Address, code []byte) {
+func (s *StateDB) SetCode(addr common.Address, code []byte) (prev []byte) {
 	stateObject := s.getOrNewStateObject(addr)
 	if stateObject != nil {
-		stateObject.SetCode(crypto.Keccak256Hash(code), code)
+		return stateObject.SetCode(crypto.Keccak256Hash(code), code)
 	}
+	return nil
 }
 
 func (s *StateDB) SetState(addr common.Address, key, value common.Hash) common.Hash {
diff --git a/core/state/statedb_hooked.go b/core/state/statedb_hooked.go
index 26d021709..31bdd06b4 100644
--- a/core/state/statedb_hooked.go
+++ b/core/state/statedb_hooked.go
@@ -182,11 +182,16 @@ func (s *hookedStateDB) SetNonce(address common.Address, nonce uint64) {
 	}
 }
 
-func (s *hookedStateDB) SetCode(address common.Address, code []byte) {
-	s.inner.SetCode(address, code)
+func (s *hookedStateDB) SetCode(address common.Address, code []byte) []byte {
+	prev := s.inner.SetCode(address, code)
 	if s.hooks.OnCodeChange != nil {
-		s.hooks.OnCodeChange(address, types.EmptyCodeHash, nil, crypto.Keccak256Hash(code), code)
+		prevHash := types.EmptyCodeHash
+		if len(prev) != 0 {
+			prevHash = crypto.Keccak256Hash(prev)
+		}
+		s.hooks.OnCodeChange(address, prevHash, prev, crypto.Keccak256Hash(code), code)
 	}
+	return prev
 }
 
 func (s *hookedStateDB) SetState(address common.Address, key common.Hash, value common.Hash) common.Hash {
diff --git a/core/state_processor_test.go b/core/state_processor_test.go
index f3d230469..e8d8c2ca2 100644
--- a/core/state_processor_test.go
+++ b/core/state_processor_test.go
@@ -63,6 +63,7 @@ func TestStateProcessorErrors(t *testing.T) {
 			TerminalTotalDifficulty: big.NewInt(0),
 			ShanghaiTime:            new(uint64),
 			CancunTime:              new(uint64),
+			PragueTime:              new(uint64),
 		}
 		signer  = types.LatestSigner(config)
 		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
@@ -110,6 +111,21 @@ func TestStateProcessorErrors(t *testing.T) {
 		}
 		return tx
 	}
+	var mkSetCodeTx = func(nonce uint64, to common.Address, gasLimit uint64, gasTipCap, gasFeeCap *big.Int, authlist []types.Authorization) *types.Transaction {
+		tx, err := types.SignTx(types.NewTx(&types.SetCodeTx{
+			Nonce:     nonce,
+			GasTipCap: uint256.MustFromBig(gasTipCap),
+			GasFeeCap: uint256.MustFromBig(gasFeeCap),
+			Gas:       gasLimit,
+			To:        to,
+			Value:     new(uint256.Int),
+			AuthList:  authlist,
+		}), signer, key1)
+		if err != nil {
+			t.Fatal(err)
+		}
+		return tx
+	}
 
 	{ // Tests against a 'recent' chain definition
 		var (
@@ -251,6 +267,13 @@ func TestStateProcessorErrors(t *testing.T) {
 				},
 				want: "could not apply tx 0 [0x6c11015985ce82db691d7b2d017acda296db88b811c3c60dc71449c76256c716]: max fee per gas less than block base fee: address 0x71562b71999873DB5b286dF957af199Ec94617F7, maxFeePerGas: 1, baseFee: 875000000",
 			},
+			{ // ErrEmptyAuthList
+				txs: []*types.Transaction{
+					mkSetCodeTx(0, common.Address{}, params.TxGas, big.NewInt(params.InitialBaseFee), big.NewInt(params.InitialBaseFee), nil),
+				},
+				want: "could not apply tx 0 [0xc18d10f4c809dbdfa1a074c3300de9bc4b7f16a20f0ec667f6f67312b71b956a]: EIP-7702 transaction with empty auth list (sender 0x71562b71999873DB5b286dF957af199Ec94617F7)",
+			},
+			// ErrSetCodeTxCreate cannot be tested: it is impossible to create a SetCode-tx with nil `to`.
 		} {
 			block := GenerateBadBlock(gspec.ToBlock(), beacon.New(ethash.NewFaker()), tt.txs, gspec.Config, false)
 			_, err := blockchain.InsertChain(types.Blocks{block})
@@ -337,7 +360,7 @@ func TestStateProcessorErrors(t *testing.T) {
 				txs: []*types.Transaction{
 					mkDynamicTx(0, common.Address{}, params.TxGas-1000, big.NewInt(0), big.NewInt(0)),
 				},
-				want: "could not apply tx 0 [0x88626ac0d53cb65308f2416103c62bb1f18b805573d4f96a3640bbbfff13c14f]: sender not an eoa: address 0x71562b71999873DB5b286dF957af199Ec94617F7, codehash: 0x9280914443471259d4570a8661015ae4a5b80186dbc619658fb494bebc3da3d1",
+				want: "could not apply tx 0 [0x88626ac0d53cb65308f2416103c62bb1f18b805573d4f96a3640bbbfff13c14f]: sender not an eoa: address 0x71562b71999873DB5b286dF957af199Ec94617F7, len(code): 4",
 			},
 		} {
 			block := GenerateBadBlock(gspec.ToBlock(), beacon.New(ethash.NewFaker()), tt.txs, gspec.Config, false)
diff --git a/core/state_transition.go b/core/state_transition.go
index ea7e3df2f..58728e470 100644
--- a/core/state_transition.go
+++ b/core/state_transition.go
@@ -67,7 +67,7 @@ func (result *ExecutionResult) Revert() []byte {
 }
 
 // IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
-func IntrinsicGas(data []byte, accessList types.AccessList, isContractCreation, isHomestead, isEIP2028, isEIP3860 bool) (uint64, error) {
+func IntrinsicGas(data []byte, accessList types.AccessList, authList []types.Authorization, isContractCreation, isHomestead, isEIP2028, isEIP3860 bool) (uint64, error) {
 	// Set the starting gas for the raw transaction
 	var gas uint64
 	if isContractCreation && isHomestead {
@@ -113,6 +113,9 @@ func IntrinsicGas(data []byte, accessList types.AccessList, isContractCreation,
 		gas += uint64(len(accessList)) * params.TxAccessListAddressGas
 		gas += uint64(accessList.StorageKeys()) * params.TxAccessListStorageKeyGas
 	}
+	if authList != nil {
+		gas += uint64(len(authList)) * params.CallNewAccountGas
+	}
 	return gas, nil
 }
 
@@ -140,6 +143,7 @@ type Message struct {
 	AccessList    types.AccessList
 	BlobGasFeeCap *big.Int
 	BlobHashes    []common.Hash
+	AuthList      []types.Authorization
 
 	// When SkipNonceChecks is true, the message nonce is not checked against the
 	// account nonce in state.
@@ -162,6 +166,7 @@ func TransactionToMessage(tx *types.Transaction, s types.Signer, baseFee *big.In
 		Value:            tx.Value(),
 		Data:             tx.Data(),
 		AccessList:       tx.AccessList(),
+		AuthList:         tx.AuthList(),
 		SkipNonceChecks:  false,
 		SkipFromEOACheck: false,
 		BlobHashes:       tx.BlobHashes(),
@@ -303,10 +308,10 @@ func (st *stateTransition) preCheck() error {
 	}
 	if !msg.SkipFromEOACheck {
 		// Make sure the sender is an EOA
-		codeHash := st.state.GetCodeHash(msg.From)
-		if codeHash != (common.Hash{}) && codeHash != types.EmptyCodeHash {
-			return fmt.Errorf("%w: address %v, codehash: %s", ErrSenderNoEOA,
-				msg.From.Hex(), codeHash)
+		code := st.state.GetCode(msg.From)
+		_, delegated := types.ParseDelegation(code)
+		if len(code) > 0 && !delegated {
+			return fmt.Errorf("%w: address %v, len(code): %d", ErrSenderNoEOA, msg.From.Hex(), len(code))
 		}
 	}
 	// Make sure that transaction gasFeeCap is greater than the baseFee (post london)
@@ -366,6 +371,15 @@ func (st *stateTransition) preCheck() error {
 			}
 		}
 	}
+	// Check that EIP-7702 authorization list signatures are well formed.
+	if msg.AuthList != nil {
+		if msg.To == nil {
+			return fmt.Errorf("%w (sender %v)", ErrSetCodeTxCreate, msg.From)
+		}
+		if len(msg.AuthList) == 0 {
+			return fmt.Errorf("%w (sender %v)", ErrEmptyAuthList, msg.From)
+		}
+	}
 	return st.buyGas()
 }
 
@@ -403,7 +417,7 @@ func (st *stateTransition) execute() (*ExecutionResult, error) {
 	)
 
 	// Check clauses 4-5, subtract intrinsic gas if everything is correct
-	gas, err := IntrinsicGas(msg.Data, msg.AccessList, contractCreation, rules.IsHomestead, rules.IsIstanbul, rules.IsShanghai)
+	gas, err := IntrinsicGas(msg.Data, msg.AccessList, msg.AuthList, contractCreation, rules.IsHomestead, rules.IsIstanbul, rules.IsShanghai)
 	if err != nil {
 		return nil, err
 	}
@@ -449,8 +463,27 @@ func (st *stateTransition) execute() (*ExecutionResult, error) {
 	if contractCreation {
 		ret, _, st.gasRemaining, vmerr = st.evm.Create(sender, msg.Data, st.gasRemaining, value)
 	} else {
-		// Increment the nonce for the next transaction
-		st.state.SetNonce(msg.From, st.state.GetNonce(sender.Address())+1)
+		// Increment the nonce for the next transaction.
+		st.state.SetNonce(msg.From, st.state.GetNonce(msg.From)+1)
+
+		// Apply EIP-7702 authorizations.
+		if msg.AuthList != nil {
+			for _, auth := range msg.AuthList {
+				// Note errors are ignored, we simply skip invalid authorizations here.
+				st.applyAuthorization(msg, &auth)
+			}
+		}
+
+		// Perform convenience warming of sender's delegation target. Although the
+		// sender is already warmed in Prepare(..), it's possible a delegation to
+		// the account was deployed during this transaction. To handle correctly,
+		// simply wait until the final state of delegations is determined before
+		// performing the resolution and warming.
+		if addr, ok := types.ParseDelegation(st.state.GetCode(*msg.To)); ok {
+			st.state.AddAddressToAccessList(addr)
+		}
+
+		// Execute the transaction's call.
 		ret, st.gasRemaining, vmerr = st.evm.Call(sender, st.to(), msg.Data, st.gasRemaining, value)
 	}
 
@@ -494,6 +527,64 @@ func (st *stateTransition) execute() (*ExecutionResult, error) {
 	}, nil
 }
 
+// validateAuthorization validates an EIP-7702 authorization against the state.
+func (st *stateTransition) validateAuthorization(auth *types.Authorization) (authority common.Address, err error) {
+	// Verify chain ID is 0 or equal to current chain ID.
+	if auth.ChainID != 0 && st.evm.ChainConfig().ChainID.Uint64() != auth.ChainID {
+		return authority, ErrAuthorizationWrongChainID
+	}
+	// Limit nonce to 2^64-1 per EIP-2681.
+	if auth.Nonce+1 < auth.Nonce {
+		return authority, ErrAuthorizationNonceOverflow
+	}
+	// Validate signature values and recover authority.
+	authority, err = auth.Authority()
+	if err != nil {
+		return authority, fmt.Errorf("%w: %v", ErrAuthorizationInvalidSignature, err)
+	}
+	// Check the authority account
+	//  1) doesn't have code or has exisiting delegation
+	//  2) matches the auth's nonce
+	//
+	// Note it is added to the access list even if the authorization is invalid.
+	st.state.AddAddressToAccessList(authority)
+	code := st.state.GetCode(authority)
+	if _, ok := types.ParseDelegation(code); len(code) != 0 && !ok {
+		return authority, ErrAuthorizationDestinationHasCode
+	}
+	if have := st.state.GetNonce(authority); have != auth.Nonce {
+		return authority, ErrAuthorizationNonceMismatch
+	}
+	return authority, nil
+}
+
+// applyAuthorization applies an EIP-7702 code delegation to the state.
+func (st *stateTransition) applyAuthorization(msg *Message, auth *types.Authorization) error {
+	authority, err := st.validateAuthorization(auth)
+	if err != nil {
+		return err
+	}
+
+	// If the account already exists in state, refund the new account cost
+	// charged in the intrinsic calculation.
+	if st.state.Exist(authority) {
+		st.state.AddRefund(params.CallNewAccountGas - params.TxAuthTupleGas)
+	}
+
+	// Update nonce and account code.
+	st.state.SetNonce(authority, auth.Nonce+1)
+	if auth.Address == (common.Address{}) {
+		// Delegation to zero address means clear.
+		st.state.SetCode(authority, nil)
+		return nil
+	}
+
+	// Otherwise install delegation to auth.Address.
+	st.state.SetCode(authority, types.AddressToDelegation(auth.Address))
+
+	return nil
+}
+
 func (st *stateTransition) refundGas(refundQuotient uint64) uint64 {
 	// Apply refund counter, capped to a refund quotient
 	refund := st.gasUsed() / refundQuotient
diff --git a/core/txpool/validation.go b/core/txpool/validation.go
index 33b383d5c..908945471 100644
--- a/core/txpool/validation.go
+++ b/core/txpool/validation.go
@@ -108,7 +108,7 @@ func ValidateTransaction(tx *types.Transaction, head *types.Header, signer types
 	}
 	// Ensure the transaction has more gas than the bare minimum needed to cover
 	// the transaction metadata
-	intrGas, err := core.IntrinsicGas(tx.Data(), tx.AccessList(), tx.To() == nil, true, opts.Config.IsIstanbul(head.Number), opts.Config.IsShanghai(head.Number, head.Time))
+	intrGas, err := core.IntrinsicGas(tx.Data(), tx.AccessList(), tx.AuthList(), tx.To() == nil, true, opts.Config.IsIstanbul(head.Number), opts.Config.IsShanghai(head.Number, head.Time))
 	if err != nil {
 		return err
 	}
diff --git a/core/types/gen_authorization.go b/core/types/gen_authorization.go
new file mode 100644
index 000000000..b598b64ff
--- /dev/null
+++ b/core/types/gen_authorization.go
@@ -0,0 +1,75 @@
+// Code generated by github.com/fjl/gencodec. DO NOT EDIT.
+
+package types
+
+import (
+	"encoding/json"
+	"errors"
+
+	"github.com/ethereum/go-ethereum/common"
+	"github.com/ethereum/go-ethereum/common/hexutil"
+	"github.com/holiman/uint256"
+)
+
+var _ = (*authorizationMarshaling)(nil)
+
+// MarshalJSON marshals as JSON.
+func (a Authorization) MarshalJSON() ([]byte, error) {
+	type Authorization struct {
+		ChainID hexutil.Uint64 `json:"chainId" gencodec:"required"`
+		Address common.Address `json:"address" gencodec:"required"`
+		Nonce   hexutil.Uint64 `json:"nonce" gencodec:"required"`
+		V       hexutil.Uint64 `json:"v" gencodec:"required"`
+		R       uint256.Int    `json:"r" gencodec:"required"`
+		S       uint256.Int    `json:"s" gencodec:"required"`
+	}
+	var enc Authorization
+	enc.ChainID = hexutil.Uint64(a.ChainID)
+	enc.Address = a.Address
+	enc.Nonce = hexutil.Uint64(a.Nonce)
+	enc.V = hexutil.Uint64(a.V)
+	enc.R = a.R
+	enc.S = a.S
+	return json.Marshal(&enc)
+}
+
+// UnmarshalJSON unmarshals from JSON.
+func (a *Authorization) UnmarshalJSON(input []byte) error {
+	type Authorization struct {
+		ChainID *hexutil.Uint64 `json:"chainId" gencodec:"required"`
+		Address *common.Address `json:"address" gencodec:"required"`
+		Nonce   *hexutil.Uint64 `json:"nonce" gencodec:"required"`
+		V       *hexutil.Uint64 `json:"v" gencodec:"required"`
+		R       *uint256.Int    `json:"r" gencodec:"required"`
+		S       *uint256.Int    `json:"s" gencodec:"required"`
+	}
+	var dec Authorization
+	if err := json.Unmarshal(input, &dec); err != nil {
+		return err
+	}
+	if dec.ChainID == nil {
+		return errors.New("missing required field 'chainId' for Authorization")
+	}
+	a.ChainID = uint64(*dec.ChainID)
+	if dec.Address == nil {
+		return errors.New("missing required field 'address' for Authorization")
+	}
+	a.Address = *dec.Address
+	if dec.Nonce == nil {
+		return errors.New("missing required field 'nonce' for Authorization")
+	}
+	a.Nonce = uint64(*dec.Nonce)
+	if dec.V == nil {
+		return errors.New("missing required field 'v' for Authorization")
+	}
+	a.V = uint8(*dec.V)
+	if dec.R == nil {
+		return errors.New("missing required field 'r' for Authorization")
+	}
+	a.R = *dec.R
+	if dec.S == nil {
+		return errors.New("missing required field 's' for Authorization")
+	}
+	a.S = *dec.S
+	return nil
+}
diff --git a/core/types/receipt.go b/core/types/receipt.go
index 4f96fde59..47f16d950 100644
--- a/core/types/receipt.go
+++ b/core/types/receipt.go
@@ -204,7 +204,7 @@ func (r *Receipt) decodeTyped(b []byte) error {
 		return errShortTypedReceipt
 	}
 	switch b[0] {
-	case DynamicFeeTxType, AccessListTxType, BlobTxType:
+	case DynamicFeeTxType, AccessListTxType, BlobTxType, SetCodeTxType:
 		var data receiptRLP
 		err := rlp.DecodeBytes(b[1:], &data)
 		if err != nil {
@@ -312,7 +312,7 @@ func (rs Receipts) EncodeIndex(i int, w *bytes.Buffer) {
 	}
 	w.WriteByte(r.Type)
 	switch r.Type {
-	case AccessListTxType, DynamicFeeTxType, BlobTxType:
+	case AccessListTxType, DynamicFeeTxType, BlobTxType, SetCodeTxType:
 		rlp.Encode(w, data)
 	default:
 		// For unsupported types, write nothing. Since this is for
diff --git a/core/types/transaction.go b/core/types/transaction.go
index ea6f5ad6b..b5fb3e2db 100644
--- a/core/types/transaction.go
+++ b/core/types/transaction.go
@@ -48,6 +48,7 @@ const (
 	AccessListTxType = 0x01
 	DynamicFeeTxType = 0x02
 	BlobTxType       = 0x03
+	SetCodeTxType    = 0x04
 )
 
 // Transaction is an Ethereum transaction.
@@ -205,6 +206,8 @@ func (tx *Transaction) decodeTyped(b []byte) (TxData, error) {
 		inner = new(DynamicFeeTx)
 	case BlobTxType:
 		inner = new(BlobTx)
+	case SetCodeTxType:
+		inner = new(SetCodeTx)
 	default:
 		return nil, ErrTxTypeNotSupported
 	}
@@ -471,6 +474,15 @@ func (tx *Transaction) WithBlobTxSidecar(sideCar *BlobTxSidecar) *Transaction {
 	return cpy
 }
 
+// AuthList returns the authorizations list of the transaction.
+func (tx *Transaction) AuthList() []Authorization {
+	setcodetx, ok := tx.inner.(*SetCodeTx)
+	if !ok {
+		return nil
+	}
+	return setcodetx.AuthList
+}
+
 // SetTime sets the decoding time of a transaction. This is used by tests to set
 // arbitrary times and by persistent transaction pools when loading old txs from
 // disk.
diff --git a/core/types/transaction_marshalling.go b/core/types/transaction_marshalling.go
index 4d5b2bcdd..4176a8220 100644
--- a/core/types/transaction_marshalling.go
+++ b/core/types/transaction_marshalling.go
@@ -43,6 +43,7 @@ type txJSON struct {
 	Input                *hexutil.Bytes  `json:"input"`
 	AccessList           *AccessList     `json:"accessList,omitempty"`
 	BlobVersionedHashes  []common.Hash   `json:"blobVersionedHashes,omitempty"`
+	AuthorizationList    []Authorization `json:"authorizationList,omitempty"`
 	V                    *hexutil.Big    `json:"v"`
 	R                    *hexutil.Big    `json:"r"`
 	S                    *hexutil.Big    `json:"s"`
@@ -153,6 +154,22 @@ func (tx *Transaction) MarshalJSON() ([]byte, error) {
 			enc.Commitments = itx.Sidecar.Commitments
 			enc.Proofs = itx.Sidecar.Proofs
 		}
+	case *SetCodeTx:
+		enc.ChainID = (*hexutil.Big)(new(big.Int).SetUint64(itx.ChainID))
+		enc.Nonce = (*hexutil.Uint64)(&itx.Nonce)
+		enc.To = tx.To()
+		enc.Gas = (*hexutil.Uint64)(&itx.Gas)
+		enc.MaxFeePerGas = (*hexutil.Big)(itx.GasFeeCap.ToBig())
+		enc.MaxPriorityFeePerGas = (*hexutil.Big)(itx.GasTipCap.ToBig())
+		enc.Value = (*hexutil.Big)(itx.Value.ToBig())
+		enc.Input = (*hexutil.Bytes)(&itx.Data)
+		enc.AccessList = &itx.AccessList
+		enc.AuthorizationList = itx.AuthList
+		enc.V = (*hexutil.Big)(itx.V.ToBig())
+		enc.R = (*hexutil.Big)(itx.R.ToBig())
+		enc.S = (*hexutil.Big)(itx.S.ToBig())
+		yparity := itx.V.Uint64()
+		enc.YParity = (*hexutil.Uint64)(&yparity)
 	}
 	return json.Marshal(&enc)
 }
@@ -409,6 +426,81 @@ func (tx *Transaction) UnmarshalJSON(input []byte) error {
 			}
 		}
 
+	case SetCodeTxType:
+		var itx SetCodeTx
+		inner = &itx
+		if dec.ChainID == nil {
+			return errors.New("missing required field 'chainId' in transaction")
+		}
+		itx.ChainID = dec.ChainID.ToInt().Uint64()
+		if dec.Nonce == nil {
+			return errors.New("missing required field 'nonce' in transaction")
+		}
+		itx.Nonce = uint64(*dec.Nonce)
+		if dec.To == nil {
+			return errors.New("missing required field 'to' in transaction")
+		}
+		itx.To = *dec.To
+		if dec.Gas == nil {
+			return errors.New("missing required field 'gas' for txdata")
+		}
+		itx.Gas = uint64(*dec.Gas)
+		if dec.MaxPriorityFeePerGas == nil {
+			return errors.New("missing required field 'maxPriorityFeePerGas' for txdata")
+		}
+		itx.GasTipCap = uint256.MustFromBig((*big.Int)(dec.MaxPriorityFeePerGas))
+		if dec.MaxFeePerGas == nil {
+			return errors.New("missing required field 'maxFeePerGas' for txdata")
+		}
+		itx.GasFeeCap = uint256.MustFromBig((*big.Int)(dec.MaxFeePerGas))
+		if dec.Value == nil {
+			return errors.New("missing required field 'value' in transaction")
+		}
+		itx.Value = uint256.MustFromBig((*big.Int)(dec.Value))
+		if dec.Input == nil {
+			return errors.New("missing required field 'input' in transaction")
+		}
+		itx.Data = *dec.Input
+		if dec.AccessList != nil {
+			itx.AccessList = *dec.AccessList
+		}
+		if dec.AuthorizationList == nil {
+			return errors.New("missing required field 'authorizationList' in transaction")
+		}
+		itx.AuthList = dec.AuthorizationList
+
+		// signature R
+		var overflow bool
+		if dec.R == nil {
+			return errors.New("missing required field 'r' in transaction")
+		}
+		itx.R, overflow = uint256.FromBig((*big.Int)(dec.R))
+		if overflow {
+			return errors.New("'r' value overflows uint256")
+		}
+		// signature S
+		if dec.S == nil {
+			return errors.New("missing required field 's' in transaction")
+		}
+		itx.S, overflow = uint256.FromBig((*big.Int)(dec.S))
+		if overflow {
+			return errors.New("'s' value overflows uint256")
+		}
+		// signature V
+		vbig, err := dec.yParityValue()
+		if err != nil {
+			return err
+		}
+		itx.V, overflow = uint256.FromBig(vbig)
+		if overflow {
+			return errors.New("'v' value overflows uint256")
+		}
+		if itx.V.Sign() != 0 || itx.R.Sign() != 0 || itx.S.Sign() != 0 {
+			if err := sanityCheckSignature(vbig, itx.R.ToBig(), itx.S.ToBig(), false); err != nil {
+				return err
+			}
+		}
+
 	default:
 		return ErrTxTypeNotSupported
 	}
diff --git a/core/types/transaction_signing.go b/core/types/transaction_signing.go
index 73011e238..78fa2fc8e 100644
--- a/core/types/transaction_signing.go
+++ b/core/types/transaction_signing.go
@@ -40,6 +40,8 @@ type sigCache struct {
 func MakeSigner(config *params.ChainConfig, blockNumber *big.Int, blockTime uint64) Signer {
 	var signer Signer
 	switch {
+	case config.IsPrague(blockNumber, blockTime):
+		signer = NewPragueSigner(config.ChainID)
 	case config.IsCancun(blockNumber, blockTime):
 		signer = NewCancunSigner(config.ChainID)
 	case config.IsLondon(blockNumber):
@@ -67,6 +69,8 @@ func LatestSigner(config *params.ChainConfig) Signer {
 	var signer Signer
 	if config.ChainID != nil {
 		switch {
+		case config.PragueTime != nil:
+			signer = NewPragueSigner(config.ChainID)
 		case config.CancunTime != nil:
 			signer = NewCancunSigner(config.ChainID)
 		case config.LondonBlock != nil:
@@ -94,7 +98,7 @@ func LatestSigner(config *params.ChainConfig) Signer {
 func LatestSignerForChainID(chainID *big.Int) Signer {
 	var signer Signer
 	if chainID != nil {
-		signer = NewCancunSigner(chainID)
+		signer = NewPragueSigner(chainID)
 	} else {
 		signer = HomesteadSigner{}
 	}
@@ -174,6 +178,77 @@ type Signer interface {
 	Equal(Signer) bool
 }
 
+type pragueSigner struct{ cancunSigner }
+
+// NewPragueSigner returns a signer that accepts
+// - EIP-7702 set code transactions
+// - EIP-4844 blob transactions
+// - EIP-1559 dynamic fee transactions
+// - EIP-2930 access list transactions,
+// - EIP-155 replay protected transactions, and
+// - legacy Homestead transactions.
+func NewPragueSigner(chainId *big.Int) Signer {
+	signer, _ := NewCancunSigner(chainId).(cancunSigner)
+	return pragueSigner{signer}
+}
+
+func (s pragueSigner) Sender(tx *Transaction) (common.Address, error) {
+	if tx.Type() != SetCodeTxType {
+		return s.cancunSigner.Sender(tx)
+	}
+	V, R, S := tx.RawSignatureValues()
+
+	// Set code txs are defined to use 0 and 1 as their recovery
+	// id, add 27 to become equivalent to unprotected Homestead signatures.
+	V = new(big.Int).Add(V, big.NewInt(27))
+	if tx.ChainId().Cmp(s.chainId) != 0 {
+		return common.Address{}, fmt.Errorf("%w: have %d want %d", ErrInvalidChainId, tx.ChainId(), s.chainId)
+	}
+	return recoverPlain(s.Hash(tx), R, S, V, true)
+}
+
+func (s pragueSigner) Equal(s2 Signer) bool {
+	x, ok := s2.(pragueSigner)
+	return ok && x.chainId.Cmp(s.chainId) == 0
+}
+
+func (s pragueSigner) SignatureValues(tx *Transaction, sig []byte) (R, S, V *big.Int, err error) {
+	txdata, ok := tx.inner.(*SetCodeTx)
+	if !ok {
+		return s.cancunSigner.SignatureValues(tx, sig)
+	}
+	// Check that chain ID of tx matches the signer. We also accept ID zero here,
+	// because it indicates that the chain ID was not specified in the tx.
+	if txdata.ChainID != 0 && new(big.Int).SetUint64(txdata.ChainID).Cmp(s.chainId) != 0 {
+		return nil, nil, nil, fmt.Errorf("%w: have %d want %d", ErrInvalidChainId, txdata.ChainID, s.chainId)
+	}
+	R, S, _ = decodeSignature(sig)
+	V = big.NewInt(int64(sig[64]))
+	return R, S, V, nil
+}
+
+// Hash returns the hash to be signed by the sender.
+// It does not uniquely identify the transaction.
+func (s pragueSigner) Hash(tx *Transaction) common.Hash {
+	if tx.Type() != SetCodeTxType {
+		return s.cancunSigner.Hash(tx)
+	}
+	return prefixedRlpHash(
+		tx.Type(),
+		[]interface{}{
+			s.chainId,
+			tx.Nonce(),
+			tx.GasTipCap(),
+			tx.GasFeeCap(),
+			tx.Gas(),
+			tx.To(),
+			tx.Value(),
+			tx.Data(),
+			tx.AccessList(),
+			tx.AuthList(),
+		})
+}
+
 type cancunSigner struct{ londonSigner }
 
 // NewCancunSigner returns a signer that accepts
diff --git a/core/types/tx_setcode.go b/core/types/tx_setcode.go
new file mode 100644
index 000000000..0fea9a63a
--- /dev/null
+++ b/core/types/tx_setcode.go
@@ -0,0 +1,226 @@
+// Copyright 2024 The go-ethereum Authors
+// This file is part of the go-ethereum library.
+//
+// The go-ethereum library is free software: you can redistribute it and/or modify
+// it under the terms of the GNU Lesser General Public License as published by
+// the Free Software Foundation, either version 3 of the License, or
+// (at your option) any later version.
+//
+// The go-ethereum library is distributed in the hope that it will be useful,
+// but WITHOUT ANY WARRANTY; without even the implied warranty of
+// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
+// GNU Lesser General Public License for more details.
+//
+// You should have received a copy of the GNU Lesser General Public License
+// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.
+
+package types
+
+import (
+	"bytes"
+	"crypto/ecdsa"
+	"errors"
+	"math/big"
+
+	"github.com/ethereum/go-ethereum/common"
+	"github.com/ethereum/go-ethereum/common/hexutil"
+	"github.com/ethereum/go-ethereum/crypto"
+	"github.com/ethereum/go-ethereum/rlp"
+	"github.com/holiman/uint256"
+)
+
+// DelegationPrefix is used by code to denote the account is delegating to
+// another account.
+var DelegationPrefix = []byte{0xef, 0x01, 0x00}
+
+// ParseDelegation tries to parse the address from a delegation slice.
+func ParseDelegation(b []byte) (common.Address, bool) {
+	if len(b) != 23 || !bytes.HasPrefix(b, DelegationPrefix) {
+		return common.Address{}, false
+	}
+	return common.BytesToAddress(b[len(DelegationPrefix):]), true
+}
+
+// AddressToDelegation adds the delegation prefix to the specified address.
+func AddressToDelegation(addr common.Address) []byte {
+	return append(DelegationPrefix, addr.Bytes()...)
+}
+
+// SetCodeTx implements the EIP-7702 transaction type which temporarily installs
+// the code at the signer's address.
+type SetCodeTx struct {
+	ChainID    uint64
+	Nonce      uint64
+	GasTipCap  *uint256.Int // a.k.a. maxPriorityFeePerGas
+	GasFeeCap  *uint256.Int // a.k.a. maxFeePerGas
+	Gas        uint64
+	To         common.Address
+	Value      *uint256.Int
+	Data       []byte
+	AccessList AccessList
+	AuthList   []Authorization
+
+	// Signature values
+	V *uint256.Int `json:"v" gencodec:"required"`
+	R *uint256.Int `json:"r" gencodec:"required"`
+	S *uint256.Int `json:"s" gencodec:"required"`
+}
+
+//go:generate go run github.com/fjl/gencodec -type Authorization -field-override authorizationMarshaling -out gen_authorization.go
+
+// Authorization is an authorization from an account to deploy code at its address.
+type Authorization struct {
+	ChainID uint64         `json:"chainId" gencodec:"required"`
+	Address common.Address `json:"address" gencodec:"required"`
+	Nonce   uint64         `json:"nonce" gencodec:"required"`
+	V       uint8          `json:"v" gencodec:"required"`
+	R       uint256.Int    `json:"r" gencodec:"required"`
+	S       uint256.Int    `json:"s" gencodec:"required"`
+}
+
+// field type overrides for gencodec
+type authorizationMarshaling struct {
+	ChainID hexutil.Uint64
+	Nonce   hexutil.Uint64
+	V       hexutil.Uint64
+}
+
+// SignAuth signs the provided authorization.
+func SignAuth(auth Authorization, prv *ecdsa.PrivateKey) (Authorization, error) {
+	sighash := auth.sigHash()
+	sig, err := crypto.Sign(sighash[:], prv)
+	if err != nil {
+		return Authorization{}, err
+	}
+	return auth.withSignature(sig), nil
+}
+
+// withSignature updates the signature of an Authorization to be equal the
+// decoded signature provided in sig.
+func (a *Authorization) withSignature(sig []byte) Authorization {
+	r, s, _ := decodeSignature(sig)
+	return Authorization{
+		ChainID: a.ChainID,
+		Address: a.Address,
+		Nonce:   a.Nonce,
+		V:       sig[64],
+		R:       *uint256.MustFromBig(r),
+		S:       *uint256.MustFromBig(s),
+	}
+}
+
+func (a *Authorization) sigHash() common.Hash {
+	return prefixedRlpHash(0x05, []any{
+		a.ChainID,
+		a.Address,
+		a.Nonce,
+	})
+}
+
+// Authority recovers the the authorizing account of an authorization.
+func (a *Authorization) Authority() (common.Address, error) {
+	sighash := a.sigHash()
+	if !crypto.ValidateSignatureValues(a.V, a.R.ToBig(), a.S.ToBig(), true) {
+		return common.Address{}, ErrInvalidSig
+	}
+	// encode the signature in uncompressed format
+	var sig [crypto.SignatureLength]byte
+	a.R.WriteToSlice(sig[:32])
+	a.S.WriteToSlice(sig[32:64])
+	sig[64] = a.V
+	// recover the public key from the signature
+	pub, err := crypto.Ecrecover(sighash[:], sig[:])
+	if err != nil {
+		return common.Address{}, err
+	}
+	if len(pub) == 0 || pub[0] != 4 {
+		return common.Address{}, errors.New("invalid public key")
+	}
+	var addr common.Address
+	copy(addr[:], crypto.Keccak256(pub[1:])[12:])
+	return addr, nil
+}
+
+// copy creates a deep copy of the transaction data and initializes all fields.
+func (tx *SetCodeTx) copy() TxData {
+	cpy := &SetCodeTx{
+		Nonce: tx.Nonce,
+		To:    tx.To,
+		Data:  common.CopyBytes(tx.Data),
+		Gas:   tx.Gas,
+		// These are copied below.
+		AccessList: make(AccessList, len(tx.AccessList)),
+		AuthList:   make([]Authorization, len(tx.AuthList)),
+		Value:      new(uint256.Int),
+		ChainID:    tx.ChainID,
+		GasTipCap:  new(uint256.Int),
+		GasFeeCap:  new(uint256.Int),
+		V:          new(uint256.Int),
+		R:          new(uint256.Int),
+		S:          new(uint256.Int),
+	}
+	copy(cpy.AccessList, tx.AccessList)
+	copy(cpy.AuthList, tx.AuthList)
+	if tx.Value != nil {
+		cpy.Value.Set(tx.Value)
+	}
+	if tx.GasTipCap != nil {
+		cpy.GasTipCap.Set(tx.GasTipCap)
+	}
+	if tx.GasFeeCap != nil {
+		cpy.GasFeeCap.Set(tx.GasFeeCap)
+	}
+	if tx.V != nil {
+		cpy.V.Set(tx.V)
+	}
+	if tx.R != nil {
+		cpy.R.Set(tx.R)
+	}
+	if tx.S != nil {
+		cpy.S.Set(tx.S)
+	}
+	return cpy
+}
+
+// accessors for innerTx.
+func (tx *SetCodeTx) txType() byte           { return SetCodeTxType }
+func (tx *SetCodeTx) chainID() *big.Int      { return big.NewInt(int64(tx.ChainID)) }
+func (tx *SetCodeTx) accessList() AccessList { return tx.AccessList }
+func (tx *SetCodeTx) data() []byte           { return tx.Data }
+func (tx *SetCodeTx) gas() uint64            { return tx.Gas }
+func (tx *SetCodeTx) gasFeeCap() *big.Int    { return tx.GasFeeCap.ToBig() }
+func (tx *SetCodeTx) gasTipCap() *big.Int    { return tx.GasTipCap.ToBig() }
+func (tx *SetCodeTx) gasPrice() *big.Int     { return tx.GasFeeCap.ToBig() }
+func (tx *SetCodeTx) value() *big.Int        { return tx.Value.ToBig() }
+func (tx *SetCodeTx) nonce() uint64          { return tx.Nonce }
+func (tx *SetCodeTx) to() *common.Address    { tmp := tx.To; return &tmp }
+
+func (tx *SetCodeTx) effectiveGasPrice(dst *big.Int, baseFee *big.Int) *big.Int {
+	if baseFee == nil {
+		return dst.Set(tx.GasFeeCap.ToBig())
+	}
+	tip := dst.Sub(tx.GasFeeCap.ToBig(), baseFee)
+	if tip.Cmp(tx.GasTipCap.ToBig()) > 0 {
+		tip.Set(tx.GasTipCap.ToBig())
+	}
+	return tip.Add(tip, baseFee)
+}
+
+func (tx *SetCodeTx) rawSignatureValues() (v, r, s *big.Int) {
+	return tx.V.ToBig(), tx.R.ToBig(), tx.S.ToBig()
+}
+
+func (tx *SetCodeTx) setSignatureValues(chainID, v, r, s *big.Int) {
+	tx.ChainID = chainID.Uint64()
+	tx.V.SetFromBig(v)
+	tx.R.SetFromBig(r)
+	tx.S.SetFromBig(s)
+}
+
+func (tx *SetCodeTx) encode(b *bytes.Buffer) error {
+	return rlp.Encode(b, tx)
+}
+
+func (tx *SetCodeTx) decode(input []byte) error {
+	return rlp.DecodeBytes(input, tx)
+}
diff --git a/core/types/tx_setcode_test.go b/core/types/tx_setcode_test.go
new file mode 100644
index 000000000..d0544573c
--- /dev/null
+++ b/core/types/tx_setcode_test.go
@@ -0,0 +1,70 @@
+// Copyright 2024 The go-ethereum Authors
+// This file is part of the go-ethereum library.
+//
+// The go-ethereum library is free software: you can redistribute it and/or modify
+// it under the terms of the GNU Lesser General Public License as published by
+// the Free Software Foundation, either version 3 of the License, or
+// (at your option) any later version.
+//
+// The go-ethereum library is distributed in the hope that it will be useful,
+// but WITHOUT ANY WARRANTY; without even the implied warranty of
+// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
+// GNU Lesser General Public License for more details.
+//
+// You should have received a copy of the GNU Lesser General Public License
+// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.
+
+package types
+
+import (
+	"testing"
+
+	"github.com/ethereum/go-ethereum/common"
+)
+
+// TestParseDelegation tests a few possible delegation designator values and
+// ensures they are parsed correctly.
+func TestParseDelegation(t *testing.T) {
+	addr := common.Address{0x42}
+	for _, tt := range []struct {
+		val  []byte
+		want *common.Address
+	}{
+		{ // simple correct delegation
+			val:  append(DelegationPrefix, addr.Bytes()...),
+			want: &addr,
+		},
+		{ // wrong address size
+			val: append(DelegationPrefix, addr.Bytes()[0:19]...),
+		},
+		{ // short address
+			val: append(DelegationPrefix, 0x42),
+		},
+		{ // long address
+			val: append(append(DelegationPrefix, addr.Bytes()...), 0x42),
+		},
+		{ // wrong prefix size
+			val: append(DelegationPrefix[:2], addr.Bytes()...),
+		},
+		{ // wrong prefix
+			val: append([]byte{0xef, 0x01, 0x01}, addr.Bytes()...),
+		},
+		{ // wrong prefix
+			val: append([]byte{0xef, 0x00, 0x00}, addr.Bytes()...),
+		},
+		{ // no prefix
+			val: addr.Bytes(),
+		},
+		{ // no address
+			val: DelegationPrefix,
+		},
+	} {
+		got, ok := ParseDelegation(tt.val)
+		if ok && tt.want == nil {
+			t.Fatalf("expected fail, got %s", got.Hex())
+		}
+		if !ok && tt.want != nil {
+			t.Fatalf("failed to parse, want %s", tt.want.Hex())
+		}
+	}
+}
diff --git a/core/verkle_witness_test.go b/core/verkle_witness_test.go
index 45b317d3c..508823120 100644
--- a/core/verkle_witness_test.go
+++ b/core/verkle_witness_test.go
@@ -83,12 +83,12 @@ var (
 func TestProcessVerkle(t *testing.T) {
 	var (
 		code                            = common.FromHex(`6060604052600a8060106000396000f360606040526008565b00`)
-		intrinsicContractCreationGas, _ = IntrinsicGas(code, nil, true, true, true, true)
+		intrinsicContractCreationGas, _ = IntrinsicGas(code, nil, nil, true, true, true, true)
 		// A contract creation that calls EXTCODECOPY in the constructor. Used to ensure that the witness
 		// will not contain that copied data.
 		// Source: https://gist.github.com/gballet/a23db1e1cb4ed105616b5920feb75985
 		codeWithExtCodeCopy                = common.FromHex(`0x60806040526040516100109061017b565b604051809103906000f08015801561002c573d6000803e3d6000fd5b506000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555034801561007857600080fd5b5060008067ffffffffffffffff8111156100955761009461024a565b5b6040519080825280601f01601f1916602001820160405280156100c75781602001600182028036833780820191505090505b50905060008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1690506020600083833c81610101906101e3565b60405161010d90610187565b61011791906101a3565b604051809103906000f080158015610133573d6000803e3d6000fd5b50600160006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff160217905550505061029b565b60d58061046783390190565b6102068061053c83390190565b61019d816101d9565b82525050565b60006020820190506101b86000830184610194565b92915050565b6000819050602082019050919050565b600081519050919050565b6000819050919050565b60006101ee826101ce565b826101f8846101be565b905061020381610279565b925060208210156102435761023e7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff8360200360080261028e565b831692505b5050919050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052604160045260246000fd5b600061028582516101d9565b80915050919050565b600082821b905092915050565b6101bd806102aa6000396000f3fe608060405234801561001057600080fd5b506004361061002b5760003560e01c8063f566852414610030575b600080fd5b61003861004e565b6040516100459190610146565b60405180910390f35b6000600160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff166381ca91d36040518163ffffffff1660e01b815260040160206040518083038186803b1580156100b857600080fd5b505afa1580156100cc573d6000803e3d6000fd5b505050506040513d601f19601f820116820180604052508101906100f0919061010a565b905090565b60008151905061010481610170565b92915050565b6000602082840312156101205761011f61016b565b5b600061012e848285016100f5565b91505092915050565b61014081610161565b82525050565b600060208201905061015b6000830184610137565b92915050565b6000819050919050565b600080fd5b61017981610161565b811461018457600080fd5b5056fea2646970667358221220a6a0e11af79f176f9c421b7b12f441356b25f6489b83d38cc828a701720b41f164736f6c63430008070033608060405234801561001057600080fd5b5060b68061001f6000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c8063ab5ed15014602d575b600080fd5b60336047565b604051603e9190605d565b60405180910390f35b60006001905090565b6057816076565b82525050565b6000602082019050607060008301846050565b92915050565b600081905091905056fea26469706673582212203a14eb0d5cd07c277d3e24912f110ddda3e553245a99afc4eeefb2fbae5327aa64736f6c63430008070033608060405234801561001057600080fd5b5060405161020638038061020683398181016040528101906100329190610063565b60018160001c6100429190610090565b60008190555050610145565b60008151905061005d8161012e565b92915050565b60006020828403121561007957610078610129565b5b60006100878482850161004e565b91505092915050565b600061009b826100f0565b91506100a6836100f0565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff038211156100db576100da6100fa565b5b828201905092915050565b6000819050919050565b6000819050919050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600080fd5b610137816100e6565b811461014257600080fd5b50565b60b3806101536000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c806381ca91d314602d575b600080fd5b60336047565b604051603e9190605a565b60405180910390f35b60005481565b6054816073565b82525050565b6000602082019050606d6000830184604d565b92915050565b600081905091905056fea26469706673582212209bff7098a2f526de1ad499866f27d6d0d6f17b74a413036d6063ca6a0998ca4264736f6c63430008070033`)
-		intrinsicCodeWithExtCodeCopyGas, _ = IntrinsicGas(codeWithExtCodeCopy, nil, true, true, true, true)
+		intrinsicCodeWithExtCodeCopyGas, _ = IntrinsicGas(codeWithExtCodeCopy, nil, nil, true, true, true, true)
 		signer                             = types.LatestSigner(testVerkleChainConfig)
 		testKey, _                         = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
 		bcdb                               = rawdb.NewMemoryDatabase() // Database for the blockchain
diff --git a/core/vm/eips.go b/core/vm/eips.go
index 71d51f81e..a51a18dc6 100644
--- a/core/vm/eips.go
+++ b/core/vm/eips.go
@@ -23,6 +23,8 @@ import (
 
 	"github.com/ethereum/go-ethereum/common"
 	"github.com/ethereum/go-ethereum/core/tracing"
+	"github.com/ethereum/go-ethereum/core/types"
+	"github.com/ethereum/go-ethereum/crypto"
 	"github.com/ethereum/go-ethereum/params"
 	"github.com/holiman/uint256"
 )
@@ -40,6 +42,7 @@ var activators = map[int]func(*JumpTable){
 	1344: enable1344,
 	1153: enable1153,
 	4762: enable4762,
+	7702: enable7702,
 }
 
 // EnableEIP enables the given EIP on the config.
@@ -703,3 +706,68 @@ func enableEOF(jt *JumpTable) {
 		memorySize:  memoryExtCall,
 	}
 }
+
+// opExtCodeCopyEIP7702 implements the EIP-7702 variation of opExtCodeCopy.
+func opExtCodeCopyEIP7702(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
+	var (
+		stack      = scope.Stack
+		a          = stack.pop()
+		memOffset  = stack.pop()
+		codeOffset = stack.pop()
+		length     = stack.pop()
+	)
+	uint64CodeOffset, overflow := codeOffset.Uint64WithOverflow()
+	if overflow {
+		uint64CodeOffset = math.MaxUint64
+	}
+	code := interpreter.evm.StateDB.GetCode(common.Address(a.Bytes20()))
+	if _, ok := types.ParseDelegation(code); ok {
+		code = types.DelegationPrefix[:2]
+	}
+	codeCopy := getData(code, uint64CodeOffset, length.Uint64())
+	scope.Memory.Set(memOffset.Uint64(), length.Uint64(), codeCopy)
+
+	return nil, nil
+}
+
+// opExtCodeSizeEIP7702 implements the EIP-7702 variation of opExtCodeSize.
+func opExtCodeSizeEIP7702(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
+	slot := scope.Stack.peek()
+	code := interpreter.evm.StateDB.GetCode(common.Address(slot.Bytes20()))
+	if _, ok := types.ParseDelegation(code); ok {
+		code = types.DelegationPrefix[:2]
+	}
+	slot.SetUint64(uint64(len(code)))
+	return nil, nil
+}
+
+// opExtCodeHashEIP7702 implements the EIP-7702 variation of opExtCodeHash.
+func opExtCodeHashEIP7702(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
+	slot := scope.Stack.peek()
+	addr := common.Address(slot.Bytes20())
+	if interpreter.evm.StateDB.Empty(addr) {
+		slot.Clear()
+		return nil, nil
+	}
+	code := interpreter.evm.StateDB.GetCode(addr)
+	if _, ok := types.ParseDelegation(code); ok {
+		// If the code is a delegation, return the prefix without version.
+		slot.SetBytes(crypto.Keccak256(types.DelegationPrefix[:2]))
+	} else {
+		// Otherwise, return normal code hash.
+		slot.SetBytes(interpreter.evm.StateDB.GetCodeHash(addr).Bytes())
+	}
+	return nil, nil
+}
+
+// enable7702 the EIP-7702 changes to support delegation designators.
+func enable7702(jt *JumpTable) {
+	jt[EXTCODECOPY].execute = opExtCodeCopyEIP7702
+	jt[EXTCODESIZE].execute = opExtCodeSizeEIP7702
+	jt[EXTCODEHASH].execute = opExtCodeHashEIP7702
+
+	jt[CALL].dynamicGas = gasCallEIP7702
+	jt[CALLCODE].dynamicGas = gasCallCodeEIP7702
+	jt[STATICCALL].dynamicGas = gasStaticCallEIP7702
+	jt[DELEGATECALL].dynamicGas = gasDelegateCallEIP7702
+}
diff --git a/core/vm/eof_validation_test.go b/core/vm/eof_validation_test.go
index 6680ca3a5..afb856a32 100644
--- a/core/vm/eof_validation_test.go
+++ b/core/vm/eof_validation_test.go
@@ -251,7 +251,7 @@ func TestValidateCode(t *testing.T) {
 			data:          make([]byte, 0),
 			subContainers: make([]*Container, 0),
 		}
-		_, err := validateCode(test.code, test.section, container, &pragueEOFInstructionSet, false)
+		_, err := validateCode(test.code, test.section, container, &eofInstructionSet, false)
 		if !errors.Is(err, test.err) {
 			t.Errorf("test %d (%s): unexpected error (want: %v, got: %v)", i, common.Bytes2Hex(test.code), test.err, err)
 		}
@@ -277,7 +277,7 @@ func BenchmarkRJUMPI(b *testing.B) {
 	}
 	b.ResetTimer()
 	for i := 0; i < b.N; i++ {
-		_, err := validateCode(code, 0, container, &pragueEOFInstructionSet, false)
+		_, err := validateCode(code, 0, container, &eofInstructionSet, false)
 		if err != nil {
 			b.Fatal(err)
 		}
@@ -309,7 +309,7 @@ func BenchmarkRJUMPV(b *testing.B) {
 	}
 	b.ResetTimer()
 	for i := 0; i < b.N; i++ {
-		_, err := validateCode(code, 0, container, &pragueEOFInstructionSet, false)
+		_, err := validateCode(code, 0, container, &pragueInstructionSet, false)
 		if err != nil {
 			b.Fatal(err)
 		}
@@ -357,7 +357,7 @@ func BenchmarkEOFValidation(b *testing.B) {
 		if err := container2.UnmarshalBinary(bin, true); err != nil {
 			b.Fatal(err)
 		}
-		if err := container2.ValidateCode(&pragueEOFInstructionSet, false); err != nil {
+		if err := container2.ValidateCode(&pragueInstructionSet, false); err != nil {
 			b.Fatal(err)
 		}
 	}
@@ -412,7 +412,7 @@ func BenchmarkEOFValidation2(b *testing.B) {
 		if err := container2.UnmarshalBinary(bin, true); err != nil {
 			b.Fatal(err)
 		}
-		if err := container2.ValidateCode(&pragueEOFInstructionSet, false); err != nil {
+		if err := container2.ValidateCode(&pragueInstructionSet, false); err != nil {
 			b.Fatal(err)
 		}
 	}
@@ -468,7 +468,7 @@ func BenchmarkEOFValidation3(b *testing.B) {
 			if err := container2.UnmarshalBinary(bin, true); err != nil {
 				b.Fatal(err)
 			}
-			if err := container2.ValidateCode(&pragueEOFInstructionSet, false); err != nil {
+			if err := container2.ValidateCode(&pragueInstructionSet, false); err != nil {
 				b.Fatal(err)
 			}
 		}
@@ -494,7 +494,7 @@ func BenchmarkRJUMPI_2(b *testing.B) {
 	}
 	b.ResetTimer()
 	for i := 0; i < b.N; i++ {
-		_, err := validateCode(code, 0, container, &pragueEOFInstructionSet, false)
+		_, err := validateCode(code, 0, container, &pragueInstructionSet, false)
 		if err != nil {
 			b.Fatal(err)
 		}
@@ -512,6 +512,6 @@ func FuzzValidate(f *testing.F) {
 	f.Fuzz(func(_ *testing.T, code []byte, maxStack uint16) {
 		var container Container
 		container.types = append(container.types, &functionMetadata{inputs: 0, outputs: 0x80, maxStackHeight: maxStack})
-		validateCode(code, 0, &container, &pragueEOFInstructionSet, true)
+		validateCode(code, 0, &container, &pragueInstructionSet, true)
 	})
 }
diff --git a/core/vm/evm.go b/core/vm/evm.go
index 07e4a272f..1a0215459 100644
--- a/core/vm/evm.go
+++ b/core/vm/evm.go
@@ -217,7 +217,7 @@ func (evm *EVM) Call(caller ContractRef, addr common.Address, input []byte, gas
 	} else {
 		// Initialise a new contract and set the code that is to be used by the EVM.
 		// The contract is a scoped environment for this execution context only.
-		code := evm.StateDB.GetCode(addr)
+		code := evm.resolveCode(addr)
 		if len(code) == 0 {
 			ret, err = nil, nil // gas is unchanged
 		} else {
@@ -225,7 +225,7 @@ func (evm *EVM) Call(caller ContractRef, addr common.Address, input []byte, gas
 			// If the account has no code, we can abort here
 			// The depth-check is already done, and precompiles handled above
 			contract := NewContract(caller, AccountRef(addrCopy), value, gas)
-			contract.SetCallCode(&addrCopy, evm.StateDB.GetCodeHash(addrCopy), code)
+			contract.SetCallCode(&addrCopy, evm.resolveCodeHash(addrCopy), code)
 			ret, err = evm.interpreter.Run(contract, input, false)
 			gas = contract.Gas
 		}
@@ -285,7 +285,7 @@ func (evm *EVM) CallCode(caller ContractRef, addr common.Address, input []byte,
 		// Initialise a new contract and set the code that is to be used by the EVM.
 		// The contract is a scoped environment for this execution context only.
 		contract := NewContract(caller, AccountRef(caller.Address()), value, gas)
-		contract.SetCallCode(&addrCopy, evm.StateDB.GetCodeHash(addrCopy), evm.StateDB.GetCode(addrCopy))
+		contract.SetCallCode(&addrCopy, evm.resolveCodeHash(addrCopy), evm.resolveCode(addrCopy))
 		ret, err = evm.interpreter.Run(contract, input, false)
 		gas = contract.Gas
 	}
@@ -332,7 +332,7 @@ func (evm *EVM) DelegateCall(caller ContractRef, addr common.Address, input []by
 		addrCopy := addr
 		// Initialise a new contract and make initialise the delegate values
 		contract := NewContract(caller, AccountRef(caller.Address()), nil, gas).AsDelegate()
-		contract.SetCallCode(&addrCopy, evm.StateDB.GetCodeHash(addrCopy), evm.StateDB.GetCode(addrCopy))
+		contract.SetCallCode(&addrCopy, evm.resolveCodeHash(addrCopy), evm.resolveCode(addrCopy))
 		ret, err = evm.interpreter.Run(contract, input, false)
 		gas = contract.Gas
 	}
@@ -387,7 +387,7 @@ func (evm *EVM) StaticCall(caller ContractRef, addr common.Address, input []byte
 		// Initialise a new contract and set the code that is to be used by the EVM.
 		// The contract is a scoped environment for this execution context only.
 		contract := NewContract(caller, AccountRef(addrCopy), new(uint256.Int), gas)
-		contract.SetCallCode(&addrCopy, evm.StateDB.GetCodeHash(addrCopy), evm.StateDB.GetCode(addrCopy))
+		contract.SetCallCode(&addrCopy, evm.resolveCodeHash(addrCopy), evm.resolveCode(addrCopy))
 		// When an error was returned by the EVM or when setting the creation code
 		// above we revert to the snapshot and consume any gas remaining. Additionally
 		// when we're in Homestead this also counts for code storage gas errors.
@@ -567,6 +567,35 @@ func (evm *EVM) Create2(caller ContractRef, code []byte, gas uint64, endowment *
 	return evm.create(caller, codeAndHash, gas, endowment, contractAddr, CREATE2)
 }
 
+// resolveCode returns the code associated with the provided account. After
+// Prague, it can also resolve code pointed to by a delegation designator.
+func (evm *EVM) resolveCode(addr common.Address) []byte {
+	code := evm.StateDB.GetCode(addr)
+	if !evm.chainRules.IsPrague {
+		return code
+	}
+	if target, ok := types.ParseDelegation(code); ok {
+		// Note we only follow one level of delegation.
+		return evm.StateDB.GetCode(target)
+	}
+	return code
+}
+
+// resolveCodeHash returns the code hash associated with the provided address.
+// After Prague, it can also resolve code hash of the account pointed to by a
+// delegation designator. Although this is not accessible in the EVM it is used
+// internally to associate jumpdest analysis to code.
+func (evm *EVM) resolveCodeHash(addr common.Address) common.Hash {
+	if evm.chainRules.IsPrague {
+		code := evm.StateDB.GetCode(addr)
+		if target, ok := types.ParseDelegation(code); ok {
+			// Note we only follow one level of delegation.
+			return evm.StateDB.GetCodeHash(target)
+		}
+	}
+	return evm.StateDB.GetCodeHash(addr)
+}
+
 // ChainConfig returns the environment's chain configuration
 func (evm *EVM) ChainConfig() *params.ChainConfig { return evm.chainConfig }
 
diff --git a/core/vm/interface.go b/core/vm/interface.go
index 9229f4d2c..011541dde 100644
--- a/core/vm/interface.go
+++ b/core/vm/interface.go
@@ -42,7 +42,9 @@ type StateDB interface {
 
 	GetCodeHash(common.Address) common.Hash
 	GetCode(common.Address) []byte
-	SetCode(common.Address, []byte)
+
+	// SetCode sets the new code for the address, and returns the previous code, if any.
+	SetCode(common.Address, []byte) []byte
 	GetCodeSize(common.Address) int
 
 	AddRefund(uint64)
diff --git a/core/vm/interpreter.go b/core/vm/interpreter.go
index c40899440..996ed6e56 100644
--- a/core/vm/interpreter.go
+++ b/core/vm/interpreter.go
@@ -109,6 +109,8 @@ func NewEVMInterpreter(evm *EVM) *EVMInterpreter {
 	case evm.chainRules.IsVerkle:
 		// TODO replace with proper instruction set when fork is specified
 		table = &verkleInstructionSet
+	case evm.chainRules.IsPrague:
+		table = &pragueInstructionSet
 	case evm.chainRules.IsCancun:
 		table = &cancunInstructionSet
 	case evm.chainRules.IsShanghai:
diff --git a/core/vm/jump_table.go b/core/vm/jump_table.go
index 658014f24..6610fa7f9 100644
--- a/core/vm/jump_table.go
+++ b/core/vm/jump_table.go
@@ -61,7 +61,8 @@ var (
 	shanghaiInstructionSet         = newShanghaiInstructionSet()
 	cancunInstructionSet           = newCancunInstructionSet()
 	verkleInstructionSet           = newVerkleInstructionSet()
-	pragueEOFInstructionSet        = newPragueEOFInstructionSet()
+	pragueInstructionSet           = newPragueInstructionSet()
+	eofInstructionSet              = newEOFInstructionSetForTesting()
 )
 
 // JumpTable contains the EVM opcodes supported at a given fork.
@@ -91,16 +92,22 @@ func newVerkleInstructionSet() JumpTable {
 	return validate(instructionSet)
 }
 
-func NewPragueEOFInstructionSetForTesting() JumpTable {
-	return newPragueEOFInstructionSet()
+func NewEOFInstructionSetForTesting() JumpTable {
+	return newEOFInstructionSetForTesting()
 }
 
-func newPragueEOFInstructionSet() JumpTable {
-	instructionSet := newCancunInstructionSet()
+func newEOFInstructionSetForTesting() JumpTable {
+	instructionSet := newPragueInstructionSet()
 	enableEOF(&instructionSet)
 	return validate(instructionSet)
 }
 
+func newPragueInstructionSet() JumpTable {
+	instructionSet := newCancunInstructionSet()
+	enable7702(&instructionSet) // EIP-7702 Setcode transaction type
+	return validate(instructionSet)
+}
+
 func newCancunInstructionSet() JumpTable {
 	instructionSet := newShanghaiInstructionSet()
 	enable4844(&instructionSet) // EIP-4844 (BLOBHASH opcode)
diff --git a/core/vm/operations_acl.go b/core/vm/operations_acl.go
index b993b651f..ff3875868 100644
--- a/core/vm/operations_acl.go
+++ b/core/vm/operations_acl.go
@@ -22,6 +22,7 @@ import (
 	"github.com/ethereum/go-ethereum/common"
 	"github.com/ethereum/go-ethereum/common/math"
 	"github.com/ethereum/go-ethereum/core/tracing"
+	"github.com/ethereum/go-ethereum/core/types"
 	"github.com/ethereum/go-ethereum/params"
 )
 
@@ -242,3 +243,70 @@ func makeSelfdestructGasFn(refundsEnabled bool) gasFunc {
 	}
 	return gasFunc
 }
+
+var (
+	gasCallEIP7702         = makeCallVariantGasCallEIP7702(gasCall)
+	gasDelegateCallEIP7702 = makeCallVariantGasCallEIP7702(gasDelegateCall)
+	gasStaticCallEIP7702   = makeCallVariantGasCallEIP7702(gasStaticCall)
+	gasCallCodeEIP7702     = makeCallVariantGasCallEIP7702(gasCallCode)
+)
+
+func makeCallVariantGasCallEIP7702(oldCalculator gasFunc) gasFunc {
+	return func(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
+		var (
+			total uint64 // total dynamic gas used
+			addr  = common.Address(stack.Back(1).Bytes20())
+		)
+
+		// Check slot presence in the access list
+		if !evm.StateDB.AddressInAccessList(addr) {
+			evm.StateDB.AddAddressToAccessList(addr)
+			// The WarmStorageReadCostEIP2929 (100) is already deducted in the form of a constant cost, so
+			// the cost to charge for cold access, if any, is Cold - Warm
+			coldCost := params.ColdAccountAccessCostEIP2929 - params.WarmStorageReadCostEIP2929
+			// Charge the remaining difference here already, to correctly calculate available
+			// gas for call
+			if !contract.UseGas(coldCost, evm.Config.Tracer, tracing.GasChangeCallStorageColdAccess) {
+				return 0, ErrOutOfGas
+			}
+			total += coldCost
+		}
+
+		// Check if code is a delegation and if so, charge for resolution.
+		if target, ok := types.ParseDelegation(evm.StateDB.GetCode(addr)); ok {
+			var cost uint64
+			if evm.StateDB.AddressInAccessList(target) {
+				cost = params.WarmStorageReadCostEIP2929
+			} else {
+				evm.StateDB.AddAddressToAccessList(target)
+				cost = params.ColdAccountAccessCostEIP2929
+			}
+			if !contract.UseGas(cost, evm.Config.Tracer, tracing.GasChangeCallStorageColdAccess) {
+				return 0, ErrOutOfGas
+			}
+			total += cost
+		}
+
+		// Now call the old calculator, which takes into account
+		// - create new account
+		// - transfer value
+		// - memory expansion
+		// - 63/64ths rule
+		old, err := oldCalculator(evm, contract, stack, mem, memorySize)
+		if err != nil {
+			return old, err
+		}
+
+		// Temporarily add the gas charge back to the contract and return value. By
+		// adding it to the return, it will be charged outside of this function, as
+		// part of the dynamic gas. This will ensure it is correctly reported to
+		// tracers.
+		contract.Gas += total
+
+		var overflow bool
+		if total, overflow = math.SafeAdd(old, total); overflow {
+			return 0, ErrGasUintOverflow
+		}
+		return total, nil
+	}
+}
diff --git a/core/vm/runtime/runtime_test.go b/core/vm/runtime/runtime_test.go
index 0e774a01c..6074e9a09 100644
--- a/core/vm/runtime/runtime_test.go
+++ b/core/vm/runtime/runtime_test.go
@@ -866,3 +866,83 @@ func BenchmarkTracerStepVsCallFrame(b *testing.B) {
 	benchmarkNonModifyingCode(10000000, code, "tracer-step-10M", stepTracer, b)
 	benchmarkNonModifyingCode(10000000, code, "tracer-call-frame-10M", callFrameTracer, b)
 }
+
+// TestDelegatedAccountAccessCost tests that calling an account with an EIP-7702
+// delegation designator incurs the correct amount of gas based on the tracer.
+func TestDelegatedAccountAccessCost(t *testing.T) {
+	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
+	statedb.SetCode(common.HexToAddress("0xff"), types.AddressToDelegation(common.HexToAddress("0xaa")))
+	statedb.SetCode(common.HexToAddress("0xaa"), program.New().Return(0, 0).Bytes())
+
+	for i, tc := range []struct {
+		code []byte
+		step int
+		want uint64
+	}{
+		{ // CALL(0xff)
+			code: []byte{
+				byte(vm.PUSH1), 0x0,
+				byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1),
+				byte(vm.PUSH1), 0xff, byte(vm.DUP1), byte(vm.CALL), byte(vm.POP),
+			},
+			step: 7,
+			want: 5455,
+		},
+		{ // CALLCODE(0xff)
+			code: []byte{
+				byte(vm.PUSH1), 0x0,
+				byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1),
+				byte(vm.PUSH1), 0xff, byte(vm.DUP1), byte(vm.CALLCODE), byte(vm.POP),
+			},
+			step: 7,
+			want: 5455,
+		},
+		{ // DELEGATECALL(0xff)
+			code: []byte{
+				byte(vm.PUSH1), 0x0,
+				byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1),
+				byte(vm.PUSH1), 0xff, byte(vm.DUP1), byte(vm.DELEGATECALL), byte(vm.POP),
+			},
+			step: 6,
+			want: 5455,
+		},
+		{ // STATICCALL(0xff)
+			code: []byte{
+				byte(vm.PUSH1), 0x0,
+				byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1),
+				byte(vm.PUSH1), 0xff, byte(vm.DUP1), byte(vm.STATICCALL), byte(vm.POP),
+			},
+			step: 6,
+			want: 5455,
+		},
+		{ // SELFDESTRUCT(0xff): should not be affected by resolution
+			code: []byte{
+				byte(vm.PUSH1), 0xff, byte(vm.SELFDESTRUCT),
+			},
+			step: 1,
+			want: 7600,
+		},
+	} {
+		var step = 0
+		var have = uint64(0)
+		Execute(tc.code, nil, &Config{
+			ChainConfig: params.MergedTestChainConfig,
+			State:       statedb,
+			EVMConfig: vm.Config{
+				Tracer: &tracing.Hooks{
+					OnOpcode: func(pc uint64, op byte, gas, cost uint64, scope tracing.OpContext, rData []byte, depth int, err error) {
+						// Uncomment to investigate failures:
+						t.Logf("%d: %v %d", step, vm.OpCode(op).String(), cost)
+						if step == tc.step {
+							have = cost
+						}
+						step++
+					},
+				},
+			},
+		})
+		if want := tc.want; have != want {
+			t.Fatalf("testcase %d, gas report wrong, step %d, have %d want %d", i, tc.step, have, want)
+		}
+	}
+}
diff --git a/eth/tracers/internal/tracetest/testdata/prestate_tracer/setcode_tx.json b/eth/tracers/internal/tracetest/testdata/prestate_tracer/setcode_tx.json
new file mode 100644
index 000000000..b7d5ee1c5
--- /dev/null
+++ b/eth/tracers/internal/tracetest/testdata/prestate_tracer/setcode_tx.json
@@ -0,0 +1,82 @@
+{
+  "genesis": {
+    "baseFeePerGas": "7",
+    "blobGasUsed": "0",
+    "difficulty": "0",
+    "excessBlobGas": "36306944",
+    "extraData": "0xd983010e00846765746888676f312e32312e308664617277696e",
+    "gasLimit": "15639172",
+    "hash": "0xc682259fda061bb9ce8ccb491d5b2d436cb73daf04e1025dd116d045ce4ad28c",
+    "miner": "0x0000000000000000000000000000000000000000",
+    "mixHash": "0xae1a5ba939a4c9ac38aabeff361169fb55a6fc2c9511457e0be6eff9514faec0",
+    "nonce": "0x0000000000000000",
+    "number": "315",
+    "parentBeaconBlockRoot": "0x0000000000000000000000000000000000000000000000000000000000000000",
+    "stateRoot": "0x577f42ab21ccfd946511c57869ace0bdf7c217c36f02b7cd3459df0ed1cffc1a",
+    "timestamp": "1709626771",
+    "withdrawals": [],
+    "withdrawalsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
+    "alloc": {
+      "0x0000000000000000000000000000000000000000": {
+        "balance": "0x272e0528"
+      },
+      "0x000000000000000000000000000000000000aaaa": {
+        "code": "0x6000600060006000600173703c4b2bd70c169f5717101caee543299fc946c75af1",
+        "balance": "0x0"
+      },
+      "0x000000000000000000000000000000000000bbbb": {
+        "code": "0x6042604255",
+        "balance": "0x0"
+      },
+      "0x703c4b2bd70c169f5717101caee543299fc946c7": {
+        "balance": "0xde0b6b3a7640000"
+      },
+      "0x71562b71999873db5b286df957af199ec94617f7": {
+        "balance": "0xde0b6b3a7640000"
+      }
+    },
+    "config": {
+      "chainId": 1337,
+      "homesteadBlock": 0,
+      "eip150Block": 0,
+      "eip155Block": 0,
+      "eip158Block": 0,
+      "byzantiumBlock": 0,
+      "constantinopleBlock": 0,
+      "petersburgBlock": 0,
+      "istanbulBlock": 0,
+      "muirGlacierBlock": 0,
+      "berlinBlock": 0,
+      "londonBlock": 0,
+      "arrowGlacierBlock": 0,
+      "grayGlacierBlock": 0,
+      "shanghaiTime": 0,
+      "cancunTime": 0,
+      "pragueTime": 0,
+      "terminalTotalDifficulty": 0
+    }
+  },
+  "context": {
+    "number": "316",
+    "difficulty": "0",
+    "timestamp": "1709626785",
+    "gasLimit": "15654443",
+    "miner": "0x0000000000000000000000000000000000000000",
+    "baseFeePerGas": "7"
+  },
+  "input": "04f90126820539800285012a05f2008307a1209471562b71999873db5b286df957af199ec94617f78080c0f8baf85c82053994000000000000000000000000000000000000aaaa0101a07ed17af7d2d2b9ba7d797a202125bf505b9a0f962a67b3b61b56783d8faf7461a001b73b6e586edc706dce6c074eaec28692fa6359fb3446a2442f36777e1c0669f85a8094000000000000000000000000000000000000bbbb8001a05011890f198f0356a887b0779bde5afa1ed04e6acb1e3f37f8f18c7b6f521b98a056c3fa3456b103f3ef4a0acb4b647b9cab9ec4bc68fbcdf1e10b49fb2bcbcf6101a0167b0ecfc343a497095c22ee4270d3cc3b971cc3599fc73bbff727e0d2ed432da01c003c72306807492bf1150e39b2f79da23b49a4e83eb6e9209ae30d3572368f",
+  "result": {
+    "0x0000000000000000000000000000000000000000": {
+      "balance": "0x272e0528"
+    },
+    "0x703c4b2bd70c169f5717101caee543299fc946c7": {
+      "balance": "0xde0b6b3a7640000",
+      "storage": {
+        "0x0000000000000000000000000000000000000000000000000000000000000042": "0x0000000000000000000000000000000000000000000000000000000000000000"
+      }
+    },
+    "0x71562b71999873db5b286df957af199ec94617f7": {
+      "balance": "0xde0b6b3a7640000"
+    }
+  }
+}
diff --git a/eth/tracers/native/prestate.go b/eth/tracers/native/prestate.go
index 9706eb43f..5776275c2 100644
--- a/eth/tracers/native/prestate.go
+++ b/eth/tracers/native/prestate.go
@@ -159,6 +159,15 @@ func (t *prestateTracer) OnTxStart(env *tracing.VMContext, tx *types.Transaction
 	t.lookupAccount(from)
 	t.lookupAccount(t.to)
 	t.lookupAccount(env.Coinbase)
+
+	// Add accounts with authorizations to the prestate before they get applied.
+	for _, auth := range tx.AuthList() {
+		addr, err := auth.Authority()
+		if err != nil {
+			continue
+		}
+		t.lookupAccount(addr)
+	}
 }
 
 func (t *prestateTracer) OnTxEnd(receipt *types.Receipt, err error) {
diff --git a/graphql/graphql.go b/graphql/graphql.go
index 6e364de6d..7af1adbb4 100644
--- a/graphql/graphql.go
+++ b/graphql/graphql.go
@@ -318,7 +318,7 @@ func (t *Transaction) MaxFeePerGas(ctx context.Context) *hexutil.Big {
 		return nil
 	}
 	switch tx.Type() {
-	case types.DynamicFeeTxType, types.BlobTxType:
+	case types.DynamicFeeTxType, types.BlobTxType, types.SetCodeTxType:
 		return (*hexutil.Big)(tx.GasFeeCap())
 	default:
 		return nil
@@ -331,7 +331,7 @@ func (t *Transaction) MaxPriorityFeePerGas(ctx context.Context) *hexutil.Big {
 		return nil
 	}
 	switch tx.Type() {
-	case types.DynamicFeeTxType, types.BlobTxType:
+	case types.DynamicFeeTxType, types.BlobTxType, types.SetCodeTxType:
 		return (*hexutil.Big)(tx.GasTipCap())
 	default:
 		return nil
diff --git a/internal/ethapi/api.go b/internal/ethapi/api.go
index de75c697e..3f9ce46dd 100644
--- a/internal/ethapi/api.go
+++ b/internal/ethapi/api.go
@@ -937,28 +937,29 @@ func RPCMarshalBlock(block *types.Block, inclTx bool, fullTx bool, config *param
 
 // RPCTransaction represents a transaction that will serialize to the RPC representation of a transaction
 type RPCTransaction struct {
-	BlockHash           *common.Hash      `json:"blockHash"`
-	BlockNumber         *hexutil.Big      `json:"blockNumber"`
-	From                common.Address    `json:"from"`
-	Gas                 hexutil.Uint64    `json:"gas"`
-	GasPrice            *hexutil.Big      `json:"gasPrice"`
-	GasFeeCap           *hexutil.Big      `json:"maxFeePerGas,omitempty"`
-	GasTipCap           *hexutil.Big      `json:"maxPriorityFeePerGas,omitempty"`
-	MaxFeePerBlobGas    *hexutil.Big      `json:"maxFeePerBlobGas,omitempty"`
-	Hash                common.Hash       `json:"hash"`
-	Input               hexutil.Bytes     `json:"input"`
-	Nonce               hexutil.Uint64    `json:"nonce"`
-	To                  *common.Address   `json:"to"`
-	TransactionIndex    *hexutil.Uint64   `json:"transactionIndex"`
-	Value               *hexutil.Big      `json:"value"`
-	Type                hexutil.Uint64    `json:"type"`
-	Accesses            *types.AccessList `json:"accessList,omitempty"`
-	ChainID             *hexutil.Big      `json:"chainId,omitempty"`
-	BlobVersionedHashes []common.Hash     `json:"blobVersionedHashes,omitempty"`
-	V                   *hexutil.Big      `json:"v"`
-	R                   *hexutil.Big      `json:"r"`
-	S                   *hexutil.Big      `json:"s"`
-	YParity             *hexutil.Uint64   `json:"yParity,omitempty"`
+	BlockHash           *common.Hash          `json:"blockHash"`
+	BlockNumber         *hexutil.Big          `json:"blockNumber"`
+	From                common.Address        `json:"from"`
+	Gas                 hexutil.Uint64        `json:"gas"`
+	GasPrice            *hexutil.Big          `json:"gasPrice"`
+	GasFeeCap           *hexutil.Big          `json:"maxFeePerGas,omitempty"`
+	GasTipCap           *hexutil.Big          `json:"maxPriorityFeePerGas,omitempty"`
+	MaxFeePerBlobGas    *hexutil.Big          `json:"maxFeePerBlobGas,omitempty"`
+	Hash                common.Hash           `json:"hash"`
+	Input               hexutil.Bytes         `json:"input"`
+	Nonce               hexutil.Uint64        `json:"nonce"`
+	To                  *common.Address       `json:"to"`
+	TransactionIndex    *hexutil.Uint64       `json:"transactionIndex"`
+	Value               *hexutil.Big          `json:"value"`
+	Type                hexutil.Uint64        `json:"type"`
+	Accesses            *types.AccessList     `json:"accessList,omitempty"`
+	ChainID             *hexutil.Big          `json:"chainId,omitempty"`
+	BlobVersionedHashes []common.Hash         `json:"blobVersionedHashes,omitempty"`
+	AuthorizationList   []types.Authorization `json:"authorizationList,omitempty"`
+	V                   *hexutil.Big          `json:"v"`
+	R                   *hexutil.Big          `json:"r"`
+	S                   *hexutil.Big          `json:"s"`
+	YParity             *hexutil.Uint64       `json:"yParity,omitempty"`
 }
 
 // newRPCTransaction returns a transaction that will serialize to the RPC
@@ -1033,6 +1034,22 @@ func newRPCTransaction(tx *types.Transaction, blockHash common.Hash, blockNumber
 		}
 		result.MaxFeePerBlobGas = (*hexutil.Big)(tx.BlobGasFeeCap())
 		result.BlobVersionedHashes = tx.BlobHashes()
+
+	case types.SetCodeTxType:
+		al := tx.AccessList()
+		yparity := hexutil.Uint64(v.Sign())
+		result.Accesses = &al
+		result.ChainID = (*hexutil.Big)(tx.ChainId())
+		result.YParity = &yparity
+		result.GasFeeCap = (*hexutil.Big)(tx.GasFeeCap())
+		result.GasTipCap = (*hexutil.Big)(tx.GasTipCap())
+		// if the transaction has been mined, compute the effective gas price
+		if baseFee != nil && blockHash != (common.Hash{}) {
+			result.GasPrice = (*hexutil.Big)(effectiveGasPrice(tx, baseFee))
+		} else {
+			result.GasPrice = (*hexutil.Big)(tx.GasFeeCap())
+		}
+		result.AuthorizationList = tx.AuthList()
 	}
 	return result
 }
diff --git a/internal/ethapi/api_test.go b/internal/ethapi/api_test.go
index ae2ec9f0f..0303a0a6e 100644
--- a/internal/ethapi/api_test.go
+++ b/internal/ethapi/api_test.go
@@ -629,12 +629,13 @@ func TestEstimateGas(t *testing.T) {
 	t.Parallel()
 	// Initialize test accounts
 	var (
-		accounts = newAccounts(2)
+		accounts = newAccounts(4)
 		genesis  = &core.Genesis{
 			Config: params.MergedTestChainConfig,
 			Alloc: types.GenesisAlloc{
 				accounts[0].addr: {Balance: big.NewInt(params.Ether)},
 				accounts[1].addr: {Balance: big.NewInt(params.Ether)},
+				accounts[2].addr: {Balance: big.NewInt(params.Ether), Code: append(types.DelegationPrefix, accounts[3].addr.Bytes()...)},
 			},
 		}
 		genBlocks      = 10
@@ -819,6 +820,26 @@ func TestEstimateGas(t *testing.T) {
 			blockOverrides: override.BlockOverrides{Number: (*hexutil.Big)(big.NewInt(11))},
 			expectErr:      newRevertError(packRevert("block 11")),
 		},
+		// Should be able to send to an EIP-7702 delegated account.
+		{
+			blockNumber: rpc.LatestBlockNumber,
+			call: TransactionArgs{
+				From:  &accounts[0].addr,
+				To:    &accounts[2].addr,
+				Value: (*hexutil.Big)(big.NewInt(1)),
+			},
+			want: 21000,
+		},
+		// Should be able to send as EIP-7702 delegated account.
+		{
+			blockNumber: rpc.LatestBlockNumber,
+			call: TransactionArgs{
+				From:  &accounts[2].addr,
+				To:    &accounts[1].addr,
+				Value: (*hexutil.Big)(big.NewInt(1)),
+			},
+			want: 21000,
+		},
 	}
 	for i, tc := range testSuite {
 		result, err := api.EstimateGas(context.Background(), tc.call, &rpc.BlockNumberOrHash{BlockNumber: &tc.blockNumber}, &tc.overrides, &tc.blockOverrides)
diff --git a/internal/ethapi/transaction_args.go b/internal/ethapi/transaction_args.go
index 2a0508b14..3942540b0 100644
--- a/internal/ethapi/transaction_args.go
+++ b/internal/ethapi/transaction_args.go
@@ -72,6 +72,9 @@ type TransactionArgs struct {
 	Commitments []kzg4844.Commitment `json:"commitments"`
 	Proofs      []kzg4844.Proof      `json:"proofs"`
 
+	// For SetCodeTxType
+	AuthorizationList []types.Authorization `json:"authorizationList"`
+
 	// This configures whether blobs are allowed to be passed.
 	blobSidecarAllowed bool
 }
@@ -473,6 +476,8 @@ func (args *TransactionArgs) ToMessage(baseFee *big.Int, skipNonceCheck, skipEoA
 func (args *TransactionArgs) ToTransaction(defaultType int) *types.Transaction {
 	usedType := types.LegacyTxType
 	switch {
+	case args.AuthorizationList != nil || defaultType == types.SetCodeTxType:
+		usedType = types.SetCodeTxType
 	case args.BlobHashes != nil || defaultType == types.BlobTxType:
 		usedType = types.BlobTxType
 	case args.MaxFeePerGas != nil || defaultType == types.DynamicFeeTxType:
@@ -486,6 +491,28 @@ func (args *TransactionArgs) ToTransaction(defaultType int) *types.Transaction {
 	}
 	var data types.TxData
 	switch usedType {
+	case types.SetCodeTxType:
+		al := types.AccessList{}
+		if args.AccessList != nil {
+			al = *args.AccessList
+		}
+		authList := []types.Authorization{}
+		if args.AuthorizationList != nil {
+			authList = args.AuthorizationList
+		}
+		data = &types.SetCodeTx{
+			To:         *args.To,
+			ChainID:    args.ChainID.ToInt().Uint64(),
+			Nonce:      uint64(*args.Nonce),
+			Gas:        uint64(*args.Gas),
+			GasFeeCap:  uint256.MustFromBig((*big.Int)(args.MaxFeePerGas)),
+			GasTipCap:  uint256.MustFromBig((*big.Int)(args.MaxPriorityFeePerGas)),
+			Value:      uint256.MustFromBig((*big.Int)(args.Value)),
+			Data:       args.data(),
+			AccessList: al,
+			AuthList:   authList,
+		}
+
 	case types.BlobTxType:
 		al := types.AccessList{}
 		if args.AccessList != nil {
diff --git a/params/protocol_params.go b/params/protocol_params.go
index 90e7487cf..4d2baf805 100644
--- a/params/protocol_params.go
+++ b/params/protocol_params.go
@@ -90,10 +90,11 @@ const (
 	SelfdestructRefundGas uint64 = 24000 // Refunded following a selfdestruct operation.
 	MemoryGas             uint64 = 3     // Times the address of the (highest referenced byte in memory + 1). NOTE: referencing happens on read, write and in instructions such as RETURN and CALL.
 
-	TxDataNonZeroGasFrontier  uint64 = 68   // Per byte of data attached to a transaction that is not equal to zero. NOTE: Not payable on data of calls between transactions.
-	TxDataNonZeroGasEIP2028   uint64 = 16   // Per byte of non zero data attached to a transaction after EIP 2028 (part in Istanbul)
-	TxAccessListAddressGas    uint64 = 2400 // Per address specified in EIP 2930 access list
-	TxAccessListStorageKeyGas uint64 = 1900 // Per storage key specified in EIP 2930 access list
+	TxDataNonZeroGasFrontier  uint64 = 68    // Per byte of data attached to a transaction that is not equal to zero. NOTE: Not payable on data of calls between transactions.
+	TxDataNonZeroGasEIP2028   uint64 = 16    // Per byte of non zero data attached to a transaction after EIP 2028 (part in Istanbul)
+	TxAccessListAddressGas    uint64 = 2400  // Per address specified in EIP 2930 access list
+	TxAccessListStorageKeyGas uint64 = 1900  // Per storage key specified in EIP 2930 access list
+	TxAuthTupleGas            uint64 = 12500 // Per auth tuple code specified in EIP-7702
 
 	// These have been changed during the course of the chain
 	CallGasFrontier              uint64 = 40  // Once per CALL operation & message call transaction.
diff --git a/tests/gen_stauthorization.go b/tests/gen_stauthorization.go
new file mode 100644
index 000000000..fbafd6fde
--- /dev/null
+++ b/tests/gen_stauthorization.go
@@ -0,0 +1,74 @@
+// Code generated by github.com/fjl/gencodec. DO NOT EDIT.
+
+package tests
+
+import (
+	"encoding/json"
+	"errors"
+	"math/big"
+
+	"github.com/ethereum/go-ethereum/common"
+	"github.com/ethereum/go-ethereum/common/math"
+)
+
+var _ = (*stAuthorizationMarshaling)(nil)
+
+// MarshalJSON marshals as JSON.
+func (s stAuthorization) MarshalJSON() ([]byte, error) {
+	type stAuthorization struct {
+		ChainID math.HexOrDecimal64
+		Address common.Address        `json:"address" gencodec:"required"`
+		Nonce   math.HexOrDecimal64   `json:"nonce" gencodec:"required"`
+		V       math.HexOrDecimal64   `json:"v" gencodec:"required"`
+		R       *math.HexOrDecimal256 `json:"r" gencodec:"required"`
+		S       *math.HexOrDecimal256 `json:"s" gencodec:"required"`
+	}
+	var enc stAuthorization
+	enc.ChainID = math.HexOrDecimal64(s.ChainID)
+	enc.Address = s.Address
+	enc.Nonce = math.HexOrDecimal64(s.Nonce)
+	enc.V = math.HexOrDecimal64(s.V)
+	enc.R = (*math.HexOrDecimal256)(s.R)
+	enc.S = (*math.HexOrDecimal256)(s.S)
+	return json.Marshal(&enc)
+}
+
+// UnmarshalJSON unmarshals from JSON.
+func (s *stAuthorization) UnmarshalJSON(input []byte) error {
+	type stAuthorization struct {
+		ChainID *math.HexOrDecimal64
+		Address *common.Address       `json:"address" gencodec:"required"`
+		Nonce   *math.HexOrDecimal64  `json:"nonce" gencodec:"required"`
+		V       *math.HexOrDecimal64  `json:"v" gencodec:"required"`
+		R       *math.HexOrDecimal256 `json:"r" gencodec:"required"`
+		S       *math.HexOrDecimal256 `json:"s" gencodec:"required"`
+	}
+	var dec stAuthorization
+	if err := json.Unmarshal(input, &dec); err != nil {
+		return err
+	}
+	if dec.ChainID != nil {
+		s.ChainID = uint64(*dec.ChainID)
+	}
+	if dec.Address == nil {
+		return errors.New("missing required field 'address' for stAuthorization")
+	}
+	s.Address = *dec.Address
+	if dec.Nonce == nil {
+		return errors.New("missing required field 'nonce' for stAuthorization")
+	}
+	s.Nonce = uint64(*dec.Nonce)
+	if dec.V == nil {
+		return errors.New("missing required field 'v' for stAuthorization")
+	}
+	s.V = uint8(*dec.V)
+	if dec.R == nil {
+		return errors.New("missing required field 'r' for stAuthorization")
+	}
+	s.R = (*big.Int)(dec.R)
+	if dec.S == nil {
+		return errors.New("missing required field 's' for stAuthorization")
+	}
+	s.S = (*big.Int)(dec.S)
+	return nil
+}
diff --git a/tests/gen_sttransaction.go b/tests/gen_sttransaction.go
index 9b5aecbfe..b25ce7616 100644
--- a/tests/gen_sttransaction.go
+++ b/tests/gen_sttransaction.go
@@ -30,6 +30,7 @@ func (s stTransaction) MarshalJSON() ([]byte, error) {
 		Sender               *common.Address       `json:"sender"`
 		BlobVersionedHashes  []common.Hash         `json:"blobVersionedHashes,omitempty"`
 		BlobGasFeeCap        *math.HexOrDecimal256 `json:"maxFeePerBlobGas,omitempty"`
+		AuthorizationList    []*stAuthorization    `json:"authorizationList,omitempty"`
 	}
 	var enc stTransaction
 	enc.GasPrice = (*math.HexOrDecimal256)(s.GasPrice)
@@ -50,6 +51,7 @@ func (s stTransaction) MarshalJSON() ([]byte, error) {
 	enc.Sender = s.Sender
 	enc.BlobVersionedHashes = s.BlobVersionedHashes
 	enc.BlobGasFeeCap = (*math.HexOrDecimal256)(s.BlobGasFeeCap)
+	enc.AuthorizationList = s.AuthorizationList
 	return json.Marshal(&enc)
 }
 
@@ -69,6 +71,7 @@ func (s *stTransaction) UnmarshalJSON(input []byte) error {
 		Sender               *common.Address       `json:"sender"`
 		BlobVersionedHashes  []common.Hash         `json:"blobVersionedHashes,omitempty"`
 		BlobGasFeeCap        *math.HexOrDecimal256 `json:"maxFeePerBlobGas,omitempty"`
+		AuthorizationList    []*stAuthorization    `json:"authorizationList,omitempty"`
 	}
 	var dec stTransaction
 	if err := json.Unmarshal(input, &dec); err != nil {
@@ -116,5 +119,8 @@ func (s *stTransaction) UnmarshalJSON(input []byte) error {
 	if dec.BlobGasFeeCap != nil {
 		s.BlobGasFeeCap = (*big.Int)(dec.BlobGasFeeCap)
 	}
+	if dec.AuthorizationList != nil {
+		s.AuthorizationList = dec.AuthorizationList
+	}
 	return nil
 }
diff --git a/tests/state_test_util.go b/tests/state_test_util.go
index 6884ae7ed..e735ce2fb 100644
--- a/tests/state_test_util.go
+++ b/tests/state_test_util.go
@@ -123,6 +123,7 @@ type stTransaction struct {
 	Sender               *common.Address     `json:"sender"`
 	BlobVersionedHashes  []common.Hash       `json:"blobVersionedHashes,omitempty"`
 	BlobGasFeeCap        *big.Int            `json:"maxFeePerBlobGas,omitempty"`
+	AuthorizationList    []*stAuthorization  `json:"authorizationList,omitempty"`
 }
 
 type stTransactionMarshaling struct {
@@ -135,6 +136,27 @@ type stTransactionMarshaling struct {
 	BlobGasFeeCap        *math.HexOrDecimal256
 }
 
+//go:generate go run github.com/fjl/gencodec -type stAuthorization -field-override stAuthorizationMarshaling -out gen_stauthorization.go
+
+// Authorization is an authorization from an account to deploy code at it's address.
+type stAuthorization struct {
+	ChainID uint64
+	Address common.Address `json:"address" gencodec:"required"`
+	Nonce   uint64         `json:"nonce" gencodec:"required"`
+	V       uint8          `json:"v" gencodec:"required"`
+	R       *big.Int       `json:"r" gencodec:"required"`
+	S       *big.Int       `json:"s" gencodec:"required"`
+}
+
+// field type overrides for gencodec
+type stAuthorizationMarshaling struct {
+	ChainID math.HexOrDecimal64
+	Nonce   math.HexOrDecimal64
+	V       math.HexOrDecimal64
+	R       *math.HexOrDecimal256
+	S       *math.HexOrDecimal256
+}
+
 // GetChainConfig takes a fork definition and returns a chain config.
 // The fork definition can be
 // - a plain forkname, e.g. `Byzantium`,
@@ -419,6 +441,20 @@ func (tx *stTransaction) toMessage(ps stPostState, baseFee *big.Int) (*core.Mess
 	if gasPrice == nil {
 		return nil, errors.New("no gas price provided")
 	}
+	var authList []types.Authorization
+	if tx.AuthorizationList != nil {
+		authList = make([]types.Authorization, len(tx.AuthorizationList))
+		for i, auth := range tx.AuthorizationList {
+			authList[i] = types.Authorization{
+				ChainID: auth.ChainID,
+				Address: auth.Address,
+				Nonce:   auth.Nonce,
+				V:       auth.V,
+				R:       *uint256.MustFromBig(auth.R),
+				S:       *uint256.MustFromBig(auth.S),
+			}
+		}
+	}
 
 	msg := &core.Message{
 		From:          from,
@@ -433,6 +469,7 @@ func (tx *stTransaction) toMessage(ps stPostState, baseFee *big.Int) (*core.Mess
 		AccessList:    accessList,
 		BlobHashes:    tx.BlobVersionedHashes,
 		BlobGasFeeCap: tx.BlobGasFeeCap,
+		AuthList:      authList,
 	}
 	return msg, nil
 }
diff --git a/tests/transaction_test_util.go b/tests/transaction_test_util.go
index d3dbbd5db..4da27ff94 100644
--- a/tests/transaction_test_util.go
+++ b/tests/transaction_test_util.go
@@ -59,7 +59,7 @@ func (tt *TransactionTest) Run(config *params.ChainConfig) error {
 			return nil, nil, err
 		}
 		// Intrinsic gas
-		requiredGas, err := core.IntrinsicGas(tx.Data(), tx.AccessList(), tx.To() == nil, isHomestead, isIstanbul, false)
+		requiredGas, err := core.IntrinsicGas(tx.Data(), tx.AccessList(), tx.AuthList(), tx.To() == nil, isHomestead, isIstanbul, false)
 		if err != nil {
 			return nil, nil, err
 		}

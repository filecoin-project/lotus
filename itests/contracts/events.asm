# https://github.com/filecoin-project/builtin-actors/blob/b1ba61053de2ceaddd5116e87823d20a8f5e38d7/actors/evm/tests/events.rs
# method dispatch:
# - 0x00000000 -> log_zero_data
# - 0x00000001 -> log_zero_nodata
# - 0x00000002 -> log_four_data
%dispatch_begin()
%dispatch(0x00, log_zero_data)
%dispatch(0x01, log_zero_nodata)
%dispatch(0x02, log_four_data)
%dispatch_end()
#### log a zero topic event with data
log_zero_data:
jumpdest
push8 0x1122334455667788
push1 0x00
mstore
push1 0x08
push1 0x18 ## index 24 into memory as mstore writes a full word
log0
push1 0x00
push1 0x00
return
#### log a zero topic event with no data
log_zero_nodata:
jumpdest
push1 0x00
push1 0x00
log0
push1 0x00
push1 0x00
return
#### log a four topic event with data
log_four_data:
jumpdest
push8 0x1122334455667788
push1 0x00
mstore
push4 0x4444
push3 0x3333
push2 0x2222
push2 0x1111
push1 0x08
push1 0x18 ## index 24 into memory as mstore writes a full word
log4
push1 0x00
push1 0x00
return

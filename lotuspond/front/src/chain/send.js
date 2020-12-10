import util from 'ipld-dag-cbor'
import { Buffer } from 'buffer'
import { Tagged } from 'borc'

async function pushMessage(client, from, inmsg) {
    if(!inmsg.Params) {
        inmsg.Params = "oA==" // 0b101_00000: empty cbor map: {}
    }
    if(!inmsg.Value) {
        inmsg.Value = "0"
    }
    if(!inmsg.Method) {
        inmsg.Method = 0
    }

/*    const msg = [
        inmsg.To,
        inmsg.From,

        inmsg.Nonce,

        inmsg.Value,

        inmsg.GasPrice,
        inmsg.GasLimit,

        inmsg.Method,
        Buffer.from(inmsg.Params, 'base64'),
    ]*/

    console.log(inmsg)

    await client.call('Filecoin.MpoolPushMessage', [inmsg, null])
}

export default pushMessage

import React from 'react';
import { Client } from 'rpc-websockets'
import Cristal from 'react-cristal'

const stateConnected = 'connected'
const stateConnecting = 'connecting'
const stateGettingToken = 'getting-token'

async function awaitListReducer(prev, c) {
  return [...await prev, await c]
}

function truncAddr(addr) {
  if (addr.length > 41) {
    return <abbr title={addr}>{addr.substr(0, 38) + '...'}</abbr>
  }
  return addr
}

class FullNode extends React.Component {
  constructor(props) {
    super(props)

    this.state = {
      state: stateGettingToken,
      id: "~",

      mining: false,
    }

    this.loadInfo = this.loadInfo.bind(this)
    this.startMining = this.startMining.bind(this)

    this.connect()
  }

  async connect() {
    console.log("gettok")

    const token = await this.props.pondClient.call('Pond.TokenFor', [this.props.node.ID])

    this.setState(() => ({
      state: stateConnecting,
      token: token,
    }))

    const client = new Client(`ws://127.0.0.1:${this.props.node.ApiPort}/rpc/v0?token=${token}`)
    client.on('open', async () => {
      this.setState(() => ({
        state: stateConnected,
        client: client,

        version: {Version: "~version~"},
        id: "~peerid~",
        peers: -1,
        balances: []
      }))

      const id = await this.state.client.call("Filecoin.ID", [])
      this.setState(() => ({id: id}))

      this.props.onConnect(client, id)

      this.loadInfo()
      setInterval(this.loadInfo, 2050)
    })

    console.log(token) // todo: use
  }

  async loadInfo() {
    const version = await this.state.client.call("Filecoin.Version", [])
    this.setState(() => ({version: version}))

    const peers = await this.state.client.call("Filecoin.NetPeers", [])
    this.setState(() => ({peers: peers.length}))

    const tipset = await this.state.client.call("Filecoin.ChainHead", [])
    this.setState(() => ({tipset: tipset}))

    const addrss = await this.state.client.call('Filecoin.WalletList', [])
    const defaultAddr = await this.state.client.call('Filecoin.WalletDefaultAddress', [])

    const balances = await addrss.map(async addr => {
      let balance = 0
      try {
        balance = await this.state.client.call('Filecoin.WalletBalance', [addr])
      } catch {
        balance = -1
      }
      return [addr, balance]
    }).reduce(awaitListReducer, Promise.resolve([]))

    this.setState(() => ({balances: balances, defaultAddr: defaultAddr}))
  }

  async startMining() {
    // TODO: Use actual miner address
    // see cli/miner.go
    this.setState({mining: true})
    await this.state.client.call("Filecoin.MinerStart", ["t0523423423"])
  }

  render() {
    let runtime = <div></div>
    if (this.state.state === stateConnected) {
      let chainInfo = <div></div>
      if (this.state.tipset !== undefined) {
        chainInfo = (
          <div>
            Head: {
            this.state.tipset.Cids.map(c => <abbr title={c['/']}>{c['/'].substr(-8)}</abbr>)
          } H:{this.state.tipset.Height}
          </div>
        )
      }

      let mine = <a href="#" disabled={this.state.mining} onClick={this.startMining}>[Mine]</a>
      if (this.state.mining) {
        mine = "[Mining]"
      }

      let balances = this.state.balances.map(([addr, balance]) => {
        let line = <span>{truncAddr(addr)}:&nbsp;{balance}&nbsp;(ActTyp)</span>
        if (this.state.defaultAddr === addr) {
          line = <b>{line}</b>
        }
        return <div>{line}</div>
      })

      runtime = (
        <div>
          <div>v{this.state.version.Version}, <abbr title={this.state.id}>{this.state.id.substr(-8)}</abbr>, {this.state.peers} peers</div>
          <div>Repo: LOTUS_PATH={this.props.node.Repo}</div>
          {chainInfo}
          {mine}
          <div>
            <div>Balances:</div>
            <div>{balances}</div>
          </div>

        </div>
      )
    }

    return (
      <Cristal
        title={"Node " + this.props.node.ID}
        initialPosition={{x: this.props.node.ID*30, y: this.props.node.ID * 30}} >
        <div className="CristalScroll">
          <div className="FullNode">
            <div>{this.props.node.ID} - {this.state.state}</div>
            {runtime}
          </div>
        </div>
      </Cristal>
    )
  }
}

export default FullNode
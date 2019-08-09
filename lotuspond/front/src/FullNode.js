import React from 'react';
import { Client } from 'rpc-websockets'
import Cristal from 'react-cristal'
import { BlockLinks } from "./BlockLink";
import StorageNodeInit from "./StorageNodeInit";

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
      mining: false,
    }

    this.loadInfo = this.loadInfo.bind(this)
    this.startMining = this.startMining.bind(this)
    this.newScepAddr = this.newScepAddr.bind(this)
    this.startStorageMiner = this.startStorageMiner.bind(this)
    this.add1k = this.add1k.bind(this)

    this.loadInfo()
    setInterval(this.loadInfo, 2050)
  }

  async loadInfo() {
    const id = await this.props.client.call("Filecoin.ID", [])

    const version = await this.props.client.call("Filecoin.Version", [])

    const peers = await this.props.client.call("Filecoin.NetPeers", [])

    const tipset = await this.props.client.call("Filecoin.ChainHead", [])

    const addrss = await this.props.client.call('Filecoin.WalletList', [])
    let defaultAddr = ""
    if (addrss.length > 0) {
      defaultAddr = await this.props.client.call('Filecoin.WalletDefaultAddress', [])
    }

    const balances = await addrss.map(async addr => {
      let balance = 0
      try {
        balance = await this.props.client.call('Filecoin.WalletBalance', [addr])
      } catch {
        balance = -1
      }
      return [addr, balance]
    }).reduce(awaitListReducer, Promise.resolve([]))

    this.setState(() => ({
      id: id,
      version: version,
      peers: peers.length,
      tipset: tipset,

      balances: balances,
      defaultAddr: defaultAddr}))
  }

  async startMining() {
    // TODO: Use actual miner address
    // see cli/miner.go
    this.setState({mining: true})
    let addr = "t0523423423" // in case we have no wallets
    if (this.state.defaultAddr) {
      addr = this.state.defaultAddr
    }

    this.setState({mining: true})
    await this.props.client.call("Filecoin.MinerStart", [addr])
  }

  async newScepAddr() {
    const t = "secp256k1"
    await this.props.client.call("Filecoin.WalletNew", [t])
    this.loadInfo()
  }

  async startStorageMiner() {
    this.props.mountWindow((onClose) => <StorageNodeInit fullRepo={this.props.node.Repo} fullConn={this.props.client} pondClient={this.props.pondClient} onClose={onClose} mountWindow={this.props.mountWindow}/>)
  }

  async add1k(to) {
    await this.props.give1k(to)
  }

  render() {
    let runtime = <div></div>

    if (this.state.id) {
      let chainInfo = <div></div>
      if (this.state.tipset !== undefined) {
        chainInfo = (
          <div>
            Head: {
            <BlockLinks cids={this.state.tipset.Cids} conn={this.state.client} mountWindow={this.props.mountWindow} />
          } H:{this.state.tipset.Height}
          </div>
        )
      }

      let mine = <a href="#" disabled={this.state.mining} onClick={this.startMining}>[Mine]</a>
      if (this.state.mining) {
        mine = "[Mining]"
      }

      let storageMine = <a href="#" onClick={this.startStorageMiner}>[Spawn Storage Miner]</a>

      let balances = this.state.balances.map(([addr, balance]) => {
        let add1k = <a href="#" onClick={() => this.add1k(addr)}>[+1k]</a>

        let line = <span>{truncAddr(addr)}:&nbsp;{balance}&nbsp;(ActTyp) {add1k}</span>
        if (this.state.defaultAddr === addr) {
          line = <b>{line}</b>
        }
        return <div key={addr}>{line}</div>
      })

      runtime = (
        <div>
          <div>{this.props.node.ID} - v{this.state.version.Version}, <abbr title={this.state.id}>{this.state.id.substr(-8)}</abbr>, {this.state.peers} peers</div>
          <div>Repo: LOTUS_PATH={this.props.node.Repo}</div>
          {chainInfo}
          <div>
            {mine} {storageMine}
          </div>
          <div>
            <div>Balances: [New <a href="#" onClick={this.newScepAddr}>[Secp256k1]</a>]</div>
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
            {runtime}
          </div>
        </div>
      </Cristal>
    )
  }
}

export default FullNode
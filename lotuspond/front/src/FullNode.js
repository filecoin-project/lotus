import React from 'react';
import { Client } from 'rpc-websockets'
import Cristal from 'react-cristal'
import { BlockLinks } from "./BlockLink";
import StorageNodeInit from "./StorageNodeInit";
import Address from "./Address";
import ChainExplorer from "./ChainExplorer";

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
    this.explorer = this.explorer.bind(this)

    this.loadInfo()
    setInterval(this.loadInfo, 2050)
  }

  async loadInfo() {
    const id = await this.props.client.call("Filecoin.ID", [])

    const version = await this.props.client.call("Filecoin.Version", [])

    const peers = await this.props.client.call("Filecoin.NetPeers", [])

    const tipset = await this.props.client.call("Filecoin.ChainHead", [])

    let addrs = await this.props.client.call('Filecoin.WalletList', [])
    let defaultAddr = ""
    if (addrs.length > 0) {
      defaultAddr = await this.props.client.call('Filecoin.WalletDefaultAddress', [])
    }
    let paychs = await this.props.client.call('Filecoin.PaychList', [])
    if(!paychs)
      paychs = []
    const vouchers = await Promise.all(paychs.map(paych => {
      return this.props.client.call('Filecoin.PaychVoucherList', [paych])
    }))

    this.setState(() => ({
      id: id,
      version: version,
      peers: peers.length,
      tipset: tipset,

      addrs: addrs,
      paychs: paychs,
      vouchers: vouchers,

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

  explorer() {
    this.props.mountWindow((onClose) => <ChainExplorer onClose={onClose} ts={this.state.tipset} client={this.props.client} mountWindow={this.props.mountWindow}/>)
  }

  render() {
    let runtime = <div></div>

    if (this.state.id) {
      let chainInfo = <div></div>
      if (this.state.tipset !== undefined) {
        chainInfo = (
          <div>
            Head: {
            <BlockLinks cids={this.state.tipset.Cids} conn={this.props.client} mountWindow={this.props.mountWindow} />
          } H:{this.state.tipset.Height} <a href="#" onClick={this.explorer}>[Explore]</a>
          </div>
        )
      }

      let mine = <a href="#" disabled={this.state.mining} onClick={this.startMining}>[Mine]</a>
      if (this.state.mining) {
        mine = "[Mining]"
      }

      let storageMine = <a href="#" onClick={this.startStorageMiner}>[Spawn Storage Miner]</a>

      let addresses = this.state.addrs.map((addr) => {
        let line = <Address client={this.props.client} add1k={this.add1k} addr={addr} mountWindow={this.props.mountWindow}/>
        if (this.state.defaultAddr === addr) {
          line = <b>{line}</b>
        }
        return <div key={addr}>{line}</div>
      })
      let paychannels = this.state.paychs.map((addr, ak) => {
        const line = <Address client={this.props.client} add1k={this.add1k} addr={addr} mountWindow={this.props.mountWindow}/>
        const vouchers = this.state.vouchers[ak].map(voucher => {
          let extra = <span></span>
          if(voucher.Extra) {
            extra = <span>Verif: &lt;<b><Address nobalance={true} client={this.props.client} addr={voucher.Extra.Actor} mountWindow={this.props.mountWindow}/>&nbsp;M{voucher.Extra.Method}</b>&gt;</span>
          }

          return <div key={voucher.Nonce} className="FullNode-voucher">
            Voucher Nonce:<b>{voucher.Nonce}</b> Lane:<b>{voucher.Lane}</b> Amt:<b>{voucher.Amount}</b> TL:<b>{voucher.TimeLock}</b> MinCl:<b>{voucher.MinCloseHeight}</b> {extra}
          </div>
        })
        return <div key={addr}>
          {line}
          {vouchers}
        </div>
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
            <div>{addresses}</div>
            <div>{paychannels}</div>
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
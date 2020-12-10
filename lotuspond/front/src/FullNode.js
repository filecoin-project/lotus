import React from 'react';
import { BlockLinks } from "./BlockLink";
import Address from "./Address";
import ChainExplorer from "./ChainExplorer";
import Client from "./Client";
import Window from "./Window";

class FullNode extends React.Component {
  constructor(props) {
    super(props)

    this.state = {}

    this.loadInfo = this.loadInfo.bind(this)
    this.newSecpAddr = this.newSecpAddr.bind(this)
    this.newBLSAddr = this.newBLSAddr.bind(this)
    this.startStorageMiner = this.startStorageMiner.bind(this)
    this.explorer = this.explorer.bind(this)
    this.client = this.client.bind(this)
    this.stop = this.stop.bind(this)

    this.loadInfo()
    let updates = setInterval(this.loadInfo, 2050)
    this.props.client.on('close', () => clearInterval(updates))
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

    let mpoolPending = (await this.props.client.call('Filecoin.MpoolPending', [tipset.Cids])).length

    this.setState(() => ({
      id: id,
      version: version,
      peers: peers.length,
      tipset: tipset,

      mpoolPending: mpoolPending,

      addrs: addrs,
      paychs: paychs,
      vouchers: vouchers,

      defaultAddr: defaultAddr,
    }))
  }

  async newSecpAddr() {
    const t = 1
    await this.props.client.call("Filecoin.WalletNew", [t])
    this.loadInfo()
  }

  async newBLSAddr() {
    const t = 2
    await this.props.client.call("Filecoin.WalletNew", [t])
    this.loadInfo()
  }

  startStorageMiner() {
    this.props.spawnStorageNode(this.props.node.Repo, this.props.client)
  }

  explorer() {
    this.props.mountWindow((onClose) => <ChainExplorer onClose={onClose} ts={this.state.tipset} client={this.props.client} mountWindow={this.props.mountWindow}/>)
  }

  client() {
    this.props.mountWindow((onClose) => <Client onClose={onClose} node={this.props.node} client={this.props.client} pondClient={this.props.pondClient} mountWindow={this.props.mountWindow}/>)
  }

  async stop() {
    await this.props.stop()
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
          } H:{this.state.tipset.Height} Mp:{this.state.mpoolPending} <a href="#" onClick={this.explorer}>[Explore]</a> <a href="#" onClick={this.client}>[Client]</a>
          </div>
        )
      }

      let storageMine = <a href="#" onClick={this.startStorageMiner} hidden={!this.props.spawnStorageNode}>[Spawn Miner]</a>

      let addresses = this.state.addrs.map((addr) => {
        let line = <Address client={this.props.client} addN={this.props.giveN} add10k={true} nonce={true} addr={addr} mountWindow={this.props.mountWindow}/>
        if (this.state.defaultAddr === addr) {
          line = <b>{line}</b>
        }
        return <div key={addr}>{line}</div>
      })
      let paychannels = this.state.paychs.map((addr, ak) => {
        const line = <Address client={this.props.client} addN={this.addN} add10k={true} addr={addr} mountWindow={this.props.mountWindow}/>
        const vouchers = this.state.vouchers[ak].map(voucher => {
          let extra = <span></span>
          if(voucher.Extra) {
            extra = <span>Verif: &lt;<b><Address nobalance={true} client={this.props.client} addr={voucher.Extra.Actor} method={voucher.Extra.Method} mountWindow={this.props.mountWindow}/></b>&gt;</span>
          }

          return <div key={`${addr} ${voucher.Lane} ${voucher.Nonce}`} className="FullNode-voucher">
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
            {storageMine}
          </div>
          <div>
            <div>Balances: [New <a href="#" onClick={this.newSecpAddr}>[Secp256k1]</a> <a href="#" onClick={this.newBLSAddr}>[BLS]</a>]</div>
            <div>{addresses}</div>
            <div>{paychannels}</div>
          </div>

        </div>
      )
    }

    let nodeID = this.props.node.ID ? this.props.node.ID : ''
    let nodePos = this.props.node.ID ? {x: this.props.node.ID*30, y: this.props.node.ID * 30} : 'center'

    return (
      <Window
        title={"Node " + nodeID}
        initialPosition={nodePos}
        initialSize={{width: 690, height: 300}}
        onClose={this.stop} >
        <div className="CristalScroll">
          <div className="FullNode">
            {runtime}
          </div>
        </div>
      </Window>
    )
  }
}

export default FullNode
import React from 'react'
import CID from 'cids'
import * as multihash from "multihashes";
import State from "./State";
import methods from "./chain/methods";

function truncAddr(addr, len) {
  if (addr.length > len) {
    return <abbr title={addr}>{addr.substr(0, len - 3) + '..'}</abbr>
  }
  return addr
}

let sheet = document.createElement('style')
document.body.appendChild(sheet);

class Address extends React.Component {
  constructor(props) {
    super(props)

    this.openState = this.openState.bind(this)

    this.state = {balance: -2}
    this.refresh = this.refresh.bind(this)
  }

  componentDidMount() {
    this.refresh()
    if(!this.props.ts) {
      this.updates = setInterval(this.refresh, 2050)
      this.props.client.on('close', () => clearInterval(this.updates))
    }
  }

  componentWillUnmount() {
    clearInterval(this.updates)
  }

  async refresh() {
    let balance = 0
    let actor = {}
    let actorInfo
    let minerInfo
    let nonce

    try {
      balance = await this.props.client.call('Filecoin.WalletBalance', [this.props.addr])
      actor = await this.props.client.call('Filecoin.StateGetActor', [this.props.addr, this.props.ts || null])

      actorInfo = await this.actorInfo(actor)
      if(this.props.miner) {
        minerInfo = await this.props.client.call('Filecoin.StateMinerPower', [this.props.addr, this.props.ts || null])
      }
      if(this.props.nonce) {
        nonce = await this.props.client.call('Filecoin.MpoolGetNonce', [this.props.addr])
      }
    } catch (err) {
      console.log(err)
      balance = -1
    }
    this.setState({balance, actor, actorInfo, minerInfo, nonce})
  }

  openState() {
    this.props.mountWindow((onClose) => <State addr={this.props.addr} actor={this.state.actor} client={this.props.client} onClose={onClose}/>)
  }

  async actorInfo(actor) {
    const c = new CID(actor.Code['/'])
    const mh = multihash.decode(c.multihash) // TODO: check identity

    let method = <span></span>
    if(this.props.method !== undefined && mh.digest.toString()) {
      method = <span>.{methods[mh.digest.toString()][this.props.method]}</span>
    }

    let info = <span>({mh.digest.toString()}{method})</span>
    switch(mh.digest.toString()) {
      case 'paych':
        const actstate = await this.props.client.call('Filecoin.StateReadState', [actor, this.props.ts || null])
        info = <span>({mh.digest.toString()}{method} to <Address nobalance={true} client={this.props.client} addr={actstate.State.To} mountWindow={this.props.mountWindow}/>)</span>
    }

    return info
  }

  add200k = async () => {
    [...Array(10).keys()].map(() => async () => await this.props.add20k(this.props.addr)).reduce(async (p, c) => [await p, await c()], Promise.resolve(null))
  }

  render() {
    let add20k = <span/>
    if(this.props.add20k) {
      add20k = <span>&nbsp;<a href="#" onClick={() => this.props.add20k(this.props.addr)}>[+20k]</a></span>
      if (this.props.add10k) {
        add20k = <span>{add20k}&nbsp;<a href="#" onClick={this.add200k}>[+200k]</a></span>
      }
    }
    let addr = truncAddr(this.props.addr, this.props.short ? 12 : 17)

    let actInfo = <span>(?)</span>
    if(this.state.balance >= 0) {
      actInfo = this.state.actorInfo
      addr = <a href="#" onClick={this.openState}>{addr}</a>
    }

    addr = <span className={`pondaddr-${this.props.addr}`}
                 onMouseEnter={() => sheet.sheet.insertRule(`.pondaddr-${this.props.addr}, .pondaddr-${this.props.addr} * { color: #11ee11 !important; }`, 0)}
                 onMouseLeave={() => sheet.sheet.deleteRule(0)}
                 >{addr}</span>

    let nonce = <span/>
    if(this.props.nonce) {
      nonce = <span>&nbsp;<abbr title={"Next nonce"}>Nc:{this.state.nonce}</abbr>{nonce}</span>
    }

    let balance = <span>:&nbsp;{this.state.balance}&nbsp;</span>
    if(this.props.nobalance) {
      balance = <span/>
    }
    if(this.props.short) {
      actInfo = <span/>
      balance = <span/>
    }

    let transfer = <span/>
    if(this.props.transfer) {
      transfer = <span>&nbsp;{this.props.transfer}FIL</span>
    }

    let minerInfo = <span/>
    if(this.state.minerInfo) {
      minerInfo = <span>&nbsp;Power: {this.state.minerInfo.MinerPower} ({this.state.minerInfo.MinerPower/this.state.minerInfo.TotalPower*100}%)</span>
    }

    return <span>{addr}{balance}{actInfo}{nonce}{add20k}{transfer}{minerInfo}</span>
  }
}

export default Address

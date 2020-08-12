import React from 'react'
import CID from 'cids'
import ReactTooltip from 'react-tooltip'
import * as multihash from "multihashes"
import State from "./State"
import methods from "./chain/methods.json"
import Fil from "./Fil";

function truncAddr(addr, len) {
  if (!addr) {
    return "<!nil>"
  }
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
      actor = await this.props.client.call('Filecoin.StateGetActor', [this.props.addr, (this.props.ts || {}).Cids])

      actorInfo = await this.actorInfo(actor, this.props.addr)
      if(this.props.miner) {
        minerInfo = await this.props.client.call('Filecoin.StateMinerPower', [this.props.addr, (this.props.ts || {}).Cids])
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
    this.props.mountWindow((onClose) => <State addr={this.props.addr} actor={this.state.actor} client={this.props.client} onClose={onClose} mountWindow={this.props.mountWindow}/>)
  }

  async actorInfo(actor, addr) {
    const c = new CID(actor.Code['/'])
    const mh = multihash.decode(c.multihash) // TODO: check identity

    let method = <span></span>
    if(this.props.method !== undefined && mh.digest.toString()) {
      method = <span>.{methods[mh.digest.toString()][this.props.method]}</span>
    }

    let info = <span>({mh.digest.toString()}{method})</span>
    switch(mh.digest.toString()) {
      case 'paych':
        const actstate = await this.props.client.call('Filecoin.StateReadState', [addr, (this.props.ts || {}).Cids])
        info = <span>({mh.digest.toString()}{method} to <Address nobalance={true} client={this.props.client} addr={actstate.State.To} mountWindow={this.props.mountWindow}/>)</span>
    }

    return info
  }

  addColl = async () => {
    const coll = await this.props.client.call('Filecoin.StatePledgeCollateral', [null])
    this.props.addN(this.props.addr, coll)
  }

  render() {
    let add20k = <span/>
    if(this.props.addN) {
      add20k = <span>&nbsp;<a href="#" onClick={() => this.props.addN(this.props.addr, 2e+18)}>[+2]</a></span>
      if (this.props.add10k) {
        add20k = <span>{add20k}&nbsp;<a href="#" onClick={() => this.props.addN(this.props.addr, 20e+18)}>[+20]</a></span>
        add20k = <span>{add20k}&nbsp;<a href="#" onClick={() => this.props.addN(this.props.addr, 200e+18)}>[+200]</a></span>
        add20k = <span>{add20k}&nbsp;<a href="#" onClick={() => this.addColl()}>[<abbr title="min collateral">+C</abbr>]</a></span>
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

    let balance = <span>:&nbsp;{<Fil>{this.state.balance}</Fil>}&nbsp;</span>
    if(this.props.nobalance) {
      balance = <span/>
    }
    if(this.props.short) {
      actInfo = <ReactTooltip id={this.props.addr} place="top" type="dark" effect="solid">{actInfo}: {<Fil>this.state.balance</Fil>}</ReactTooltip>
      balance = <span/>
    }

    let transfer = <span/>
    if(this.props.transfer) {
      transfer = <span>&nbsp;<Fil>{this.props.transfer}</Fil>FIL</span>
    }

    let minerInfo = <span/>
    if(this.state.minerInfo) {
      minerInfo = <span>&nbsp;Power: {this.state.minerInfo.MinerPower.QualityAdjPower} ({this.state.minerInfo.MinerPower.QualityAdjPower/this.state.minerInfo.TotalPower.QualityAdjPower*100}%)</span>
    }

    return <span data-tip data-for={this.props.addr}>{addr}{balance}{actInfo}{nonce}{add20k}{transfer}{minerInfo}</span>
  }
}

export default Address

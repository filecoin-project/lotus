import React from 'react'
import CID from 'cids'
import * as multihash from "multihashes";
import State from "./State";

function truncAddr(addr) {
  if (addr.length > 21) {
    return <abbr title={addr}>{addr.substr(0, 18) + '..'}</abbr>
  }
  return addr
}

class Address extends React.Component {
  constructor(props) {
    super(props)

    this.openState = this.openState.bind(this)

    this.state = {balance: -2}
    this.refresh = this.refresh.bind(this)
  }

  componentDidMount() {
    this.refresh()
    if(!this.props.ts)
      setInterval(this.refresh, 2050)
  }

  async refresh() {
    let balance = 0
    let actor = {}
    let actorInfo

    try {
      balance = await this.props.client.call('Filecoin.WalletBalance', [this.props.addr])
      actor = await this.props.client.call('Filecoin.ChainGetActor', [this.props.addr, this.props.ts || null])
      actorInfo = await this.actorInfo(actor)
    } catch (err) {
      console.log(err)
      balance = -1
    }
    this.setState({balance, actor, actorInfo})
  }

  openState() {
    this.props.mountWindow((onClose) => <State addr={this.props.addr} actor={this.state.actor} client={this.props.client} onClose={onClose}/>)
  }

  async actorInfo(actor) {
    const c = new CID(actor.Code['/'])
    const mh = multihash.decode(c.multihash) // TODO: check identity

    let info = <span>({mh.digest.toString()})</span>
    switch(mh.digest.toString()) {
      case 'paych':
        const actstate = await this.props.client.call('Filecoin.ChainReadState', [actor, this.props.ts || null])
        info = <span>({mh.digest.toString()} to <Address nobalance={true} client={this.props.client} addr={actstate.State.To} mountWindow={this.props.mountWindow}/>)</span>
    }

    return info
  }

  render() {
    let add1k = <span/>
    if(this.props.add1k) {
      add1k = <span>&nbsp;<a href="#" onClick={() => this.props.add1k(this.props.addr)}>[+1k]</a></span>
    }
    let addr = truncAddr(this.props.addr)

    let actInfo = <span>(?)</span>
    if(this.state.balance >= 0) {
      actInfo = this.state.actorInfo
      addr = <a href="#" onClick={this.openState}>{addr}</a>
    }

    let balance = <span>:&nbsp;{this.state.balance}</span>
    if(this.props.nobalance) {
      balance = <span></span>
    }

    return <span>{addr}{balance}&nbsp;{actInfo}{add1k}</span>
  }
}

export default Address

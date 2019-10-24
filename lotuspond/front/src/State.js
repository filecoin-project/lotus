import React from 'react'
import Window from "./Window";
import CID from "cids";
import * as multihash from "multihashes";
import code from "./chain/code";
import Address from "./Address";

class State extends React.Component {
  byCode = {
    [code.init]: InitState,
    [code.power]: PowerState,
    [code.market]: MarketState,
  }

  constructor(props) {
    super(props)

    this.state = {Balance: -2, State: {}}
  }

  async componentDidMount() {
    const tipset = this.props.tipset || await this.props.client.call("Filecoin.ChainHead", [])
    const actstate = await this.props.client.call('Filecoin.StateReadState', [this.props.actor, tipset])

    const c = new CID(this.props.actor.Code['/'])
    const mh = multihash.decode(c.multihash)
    let code = mh.digest.toString()

    this.setState({...actstate, code: code})
  }

  render() {
    let state
    if(this.byCode[this.state.code]) {
      const Stelem = this.byCode[this.state.code]
      state = <Stelem client={this.props.client} mountWindow={this.props.mountWindow} tipset={this.props.tipset}/>
    } else {
      state = <div>{Object.keys(this.state.State).map(k => <div key={k}>{k}: <span>{JSON.stringify(this.state.State[k])}</span></div>)}</div>
    }

    const content = <div className="State">
      <div>Balance: {this.state.Balance}</div>
      <div>---</div>
      {state}
    </div>
    return <Window initialSize={{width: 700, height: 400}} onClose={this.props.onClose} title={`Actor ${this.props.addr} ${this.props.tipset && this.props.tipset.Height || ''} ${this.state.code}`}>
      {content}
    </Window>
  }
}

class InitState extends React.Component {
  constructor(props) {
    super(props)

    this.state = {actors: []}
  }

  async componentDidMount() {
    const tipset = await this.props.client.call("Filecoin.ChainHead", []) // TODO: from props
    const actors = await this.props.client.call("Filecoin.StateListActors", [tipset])
    this.setState({actors: actors})
  }

  render() {
    return this.state.actors.sort((a, b) => (Number(a.substr(1)) > Number(b.substr(1))))
      .map(addr => <div key={addr}><Address addr={addr} client={this.props.client} mountWindow={this.props.mountWindow}/></div>)
  }
}

class PowerState extends React.Component {
  constructor(props) {
    super(props)

    this.state = {actors: []}
  }

  async componentDidMount() {
    const tipset = await this.props.client.call("Filecoin.ChainHead", []) // TODO: from props
    const actors = await this.props.client.call("Filecoin.StateListMiners", [tipset])
    this.setState({actors: actors})
  }

  render() {
    return this.state.actors.sort((a, b) => (Number(a.substr(1)) > Number(b.substr(1))))
      .map(addr => <div key={addr}><Address miner={true} addr={addr} client={this.props.client} mountWindow={this.props.mountWindow}/></div>)
  }
}

class MarketState extends React.Component {
  constructor(props) {
    super(props)
    this.state = {participants: {}, deals: []}
  }

  async componentDidMount() {
    const tipset = await this.props.client.call("Filecoin.ChainHead", []) // TODO: from props
    const participants = await this.props.client.call("Filecoin.StateMarketParticipants", [tipset])
    const deals = await this.props.client.call("Filecoin.StateMarketDeals", [tipset])
    this.setState({participants, deals})
  }

  render() {
    return <div>
      <div>
        <div>Participants:</div>
        <table>
          <tr><td>Address</td><td>Available</td><td>Locked</td></tr>
          {Object.keys(this.state.participants).map(p => <tr>
            <td><Address addr={p} client={this.props.client} mountWindow={this.props.mountWindow}/></td>
            <td>{this.state.participants[p].Available}</td>
            <td>{this.state.participants[p].Locked}</td>
          </tr>)}
        </table>
      </div>
      <div>
        <div>---</div>
        <div>Deals:</div>
        <table>
          <tr><td>id</td><td>Active</td><td>Client</td><td>Provider</td><td>Size</td><td>Price</td><td>Duration</td></tr>
          {Object.keys(this.state.deals).map(d => <tr>
            <td>{d}</td>
            <td>{this.state.deals[d].ActivationEpoch || "No"}</td>
            <td><Address short={true} addr={this.state.deals[d].Deal.Proposal.Provider} client={this.props.client} mountWindow={this.props.mountWindow}/></td>
            <td><Address short={true} addr={this.state.deals[d].Deal.Proposal.Client} client={this.props.client} mountWindow={this.props.mountWindow}/></td>
            <td>{this.state.deals[d].Deal.Proposal.PieceSize}B</td>
            <td>{this.state.deals[d].Deal.Proposal.StoragePrice}</td>
            <td>{this.state.deals[d].Deal.Proposal.Duration}</td>
          </tr>)}
        </table>
      </div>
    </div>
  }
}

export default State
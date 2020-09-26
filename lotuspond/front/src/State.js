import React from 'react'
import Window from "./Window";
import CID from "cids";
import * as multihash from "multihashes";
import code from "./chain/code";
import Address from "./Address";
import Fil from "./Fil";

class State extends React.Component {
  byCode = {
    [code.init]: InitState,
    [code.power]: PowerState,
    [code.market]: MarketState,
    [code.miner]: MinerState,
  }

  constructor(props) {
    super(props)

    this.state = {Balance: -2, State: {}}
  }

  async componentDidMount() {
    const tipset = this.props.tipset || await this.props.client.call("Filecoin.ChainHead", [])
    const actstate = await this.props.client.call('Filecoin.StateReadState', [this.props.addr, tipset.Cids])

    const c = new CID(this.props.actor.Code['/'])
    const mh = multihash.decode(c.multihash)
    let code = mh.digest.toString()

    this.setState({...actstate, code: code})
  }

  render() {
    let state
    if(this.byCode[this.state.code]) {
      const Stelem = this.byCode[this.state.code]
      state = <Stelem addr={this.props.addr} actor={this.props.actor} client={this.props.client} mountWindow={this.props.mountWindow} tipset={this.props.tipset}/>
    } else {
      state = <div>{Object.keys(this.state.State || {}).map(k => <div key={k}>{k}: <span>{JSON.stringify(this.state.State[k])}</span></div>)}</div>
    }

    const content = <div className="State">
      <div>Balance: <Fil>{this.state.Balance}</Fil></div>
      <div>---</div>
      {state}
    </div>
    return <Window initialSize={{width: 850, height: 400}} onClose={this.props.onClose} title={`Actor ${this.props.addr} ${this.props.tipset && this.props.tipset.Height || ''} ${this.state.code}`}>
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
    const actors = await this.props.client.call("Filecoin.StateListActors", [tipset.Cids])
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

    this.state = {actors: [], state: {State: {}}}
  }

  async componentDidMount() {
    const tipset = await this.props.client.call("Filecoin.ChainHead", []) // TODO: from props
    const actors = await this.props.client.call("Filecoin.StateListMiners", [tipset.Cids])
    const state = await this.props.client.call('Filecoin.StateReadState', [this.props.addr, tipset.Cids])

    this.setState({actors, state})
  }

  render() {
    return <div>
      <div>
        <div>Total Power: <b>{this.state.state.State.TotalStorage}</b></div>
      </div>
      <div>---</div>
      <div>{this.state.actors.sort((a, b) => (Number(a.substr(1)) > Number(b.substr(1))))
        .map(addr => <div key={addr}><Address miner={true} addr={addr} client={this.props.client} mountWindow={this.props.mountWindow}/></div>)}</div>
    </div>
  }
}

class MarketState extends React.Component {
  constructor(props) {
    super(props)
    this.state = {participants: {}, deals: []}
  }

  async componentDidMount() {
    const tipset = await this.props.client.call("Filecoin.ChainHead", []) // TODO: from props
    const participants = await this.props.client.call("Filecoin.StateMarketParticipants", [tipset.Cids])
    const deals = await this.props.client.call("Filecoin.StateMarketDeals", [tipset.Cids])
    const state = await this.props.client.call('Filecoin.StateReadState', [this.props.addr, tipset.Cids])
    this.setState({participants, deals, nextDeal: state.State.NextDealID})
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
        <div>Deals ({this.state.nextDeal} Total):</div>
        <table>
          <tr><td>id</td><td>Started</td><td>Client</td><td>Provider</td><td>Size</td><td>Price</td><td>Duration</td></tr>
          {Object.keys(this.state.deals).map(d => <tr>
            <td>{d}</td>
            <td>{this.state.deals[d].State.SectorStartEpoch || "No"}</td>
            <td><Address short={true} addr={this.state.deals[d].Proposal.Client} client={this.props.client} mountWindow={this.props.mountWindow}/></td>
            <td><Address short={true} addr={this.state.deals[d].Proposal.Provider} client={this.props.client} mountWindow={this.props.mountWindow}/></td>
            <td>{this.state.deals[d].Proposal.PieceSize}B</td>
            <td>{this.state.deals[d].Proposal.StoragePricePerEpoch*(this.state.deals[d].Proposal.EndEpoch-this.state.deals[d].Proposal.StartEpoch)}</td>
            <td>{this.state.deals[d].Proposal.EndEpoch-this.state.deals[d].Proposal.StartEpoch}</td>
          </tr>)}
        </table>
      </div>
    </div>
  }
}

class MinerState extends React.Component {
  constructor(props) {
    super(props)
    this.state = {state: {}, sectorSize: -1, worker: "", networkPower: 0, sectors: {}}
  }

  async componentDidMount() {
    const tipset = await this.props.client.call("Filecoin.ChainHead", []) // TODO: from props

    const state = await this.props.client.call('Filecoin.StateReadState', [this.props.addr, tipset.Cids])
    const sectorSize = await this.props.client.call("Filecoin.StateMinerSectorSize", [this.props.addr, tipset.Cids])
    const worker = await this.props.client.call("Filecoin.StateMinerWorker", [this.props.addr, tipset.Cids])

    const tpow = await this.props.client.call("Filecoin.StateMinerPower", [this.props.addr, tipset.Cids])
    const networkPower = tpow.TotalPower

    let sectors = {}

    const sset = await this.props.client.call("Filecoin.StateMinerSectors", [this.props.addr, tipset.Cids]) || []
    const pset = await this.props.client.call("Filecoin.StateMinerProvingSet", [this.props.addr, tipset.Cids]) || []

    sset.forEach(s => sectors[s.SectorID] = {...s, sectorSet: true})
    pset.forEach(s => sectors[s.SectorID] = {...(sectors[s.SectorID] || s), provingSet: true})

    this.setState({state, sectorSize, worker, networkPower, sectors})
  }

  render() {
    if (!this.state.worker) {
      return <span>(...)</span>
    }

    let state = this.state.state.State

    return <div>
      <div>Worker: <Address addr={this.state.worker} client={this.props.client} mountWindow={this.props.mountWindow}/></div>
      <div>Sector Size: <b>{this.state.sectorSize/1024}</b> KiB</div>
      <div>Power: <b>todoPower</b> (<b>{1/this.state.networkPower*100}</b>%)</div>
      <div>Election Period Start: <b>{state.ElectionPeriodStart}</b></div>
      <div>Slashed: <b>{state.SlashedAt === 0 ? "NO" : state.SlashedAt}</b></div>
      <div>
        <div>----</div>
        <div>Sectors:</div>
        <table style={{overflowY: "scroll"}}>
          <thead>
            <tr><td>ID</td><td>CommD</td><td>CommR</td><td>SectorSet</td><td>Proving</td></tr>
          </thead>
          <tbody>
            {Object.keys(this.state.sectors).map(sid => <tr key={sid} style={{whiteSpace: 'nowrap'}}>
              <td>{sid}</td>
              <td>{this.state.sectors[sid].CommD}</td>
              <td>{this.state.sectors[sid].CommR}</td>
              <td>{this.state.sectors[sid].sectorSet ? 'X' : ' '}</td>
              <td>{this.state.sectors[sid].provingSet ? 'X' : ' '}</td>
            </tr>)}
          </tbody>
        </table>
      </div>
    </div>
  }
}

export default State
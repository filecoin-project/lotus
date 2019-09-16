import React from 'react';
import {Cristal} from "react-cristal";
import { Client } from 'rpc-websockets'
import Address from "./Address";

const stateConnected = 'connected'
const stateConnecting = 'connecting'
const stateGettingToken = 'getting-token'

let sealCodes = [
  'Sealed',
  'Pending',
  'Failed',
  'Sealing'
]

class StorageNode extends React.Component {
  constructor(props) {
    super(props)

    this.state = {
      state: stateGettingToken,
      id: "~",

      mining: false,

      statusCounts: [0, 0, 0, 0]
    }

    this.loadInfo = this.loadInfo.bind(this)
    this.sealGarbage = this.sealGarbage.bind(this)

    this.connect()
  }

  async connect() {
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

      // this.props.onConnect(client, id) // TODO: dedupe connecting part

      this.loadInfo()
      setInterval(this.loadInfo, 1050)
    })

    console.log(token) // todo: use
  }

  async loadInfo() {
    const version = await this.state.client.call("Filecoin.Version", [])
    const peers = await this.state.client.call("Filecoin.NetPeers", [])
    const [actor] = await this.state.client.call("Filecoin.ActorAddresses", [])

    this.setState({version: version, peers: peers.length, actor: actor})
    await this.stagedList()
  }

  async stagedList() {
    let stagedList = await this.state.client.call("Filecoin.SectorsStagedList", [])
    let staged = await stagedList
      .map(sector => this.state.client.call("Filecoin.SectorsStatus", [sector.SectorID]))
      .reduce(async (p, n) => [...await p, await n], Promise.resolve([]))

    let statusCounts = staged.reduce((p, n) => p.map((e, i) => e + (i === n.SealStatusCode ? 1 : 0) ), [0, 0, 0, 0])

    this.setState({staged, statusCounts})
  }

  async sealGarbage() {
    await this.state.client.call("Filecoin.StoreGarbageData", [])
  }

  render() {
    let runtime = <div></div>
    if (this.state.actor) {
      const sealGarbage = <a href="#" onClick={this.sealGarbage}>[Seal Garbage]</a>

      runtime = (
        <div>
          <div>v{this.state.version.Version}, <abbr title={this.state.id}>{this.state.id.substr(-8)}</abbr>, {this.state.peers} peers</div>
          <div>Repo: LOTUS_STORAGE_PATH={this.props.node.Repo}</div>
          <div>
            {sealGarbage}
          </div>
          <div>
            <Address client={this.props.fullConn} addr={this.state.actor} mountWindow={this.props.mountWindow}/>
          </div>
          <div>{this.state.statusCounts.map((c, i) => <span key={i}>{sealCodes[i]}: {c} | </span>)}</div>
          <div>
            {this.state.staged ? this.state.staged.map((s, i) => (
              <div key={i}>{s.SectorID} {sealCodes[s.SealStatusCode]}</div>
            )) : <div/>}
          </div>

        </div>
      )
    }

    return <Cristal
      title={"Storage Miner Node " + this.props.node.ID}
      initialPosition={{x: this.props.node.ID*30, y: this.props.node.ID * 30}}>
      <div className="CristalScroll">
        <div className="StorageNode">
          {runtime}
        </div>
      </div>
    </Cristal>
  }
}

export default StorageNode
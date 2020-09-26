import React from 'react';
import { Client } from 'rpc-websockets'
import Address from "./Address";
import Window from "./Window";

const stateConnected = 'connected'
const stateConnecting = 'connecting'
const stateGettingToken = 'getting-token'

let sealCodes = [
 "UndefinedSectorState",
 "Empty",
 "Packing",
 "Unsealed",
 "PreCommitting",
 "WaitSeed",
 "Committing",
 "CommitWait",
 "FinalizeSector",
 "Proving",

 "SealFailed",
 "PreCommitFailed",
 "SealCommitFailed",
 "CommitFailed",
 "PackingFailed",

 "FailedUnrecoverable",
 "Faulty",
 "FaultReported",
 "FaultedFinal",
]

class StorageNode extends React.Component {
  constructor(props) {
    super(props)

    this.state = {
      state: stateGettingToken,
      id: "~",

      mining: false,

      statusCounts: [0, 0, 0, 0, 0]
    }

    this.loadInfo = this.loadInfo.bind(this)
    this.pledgeSector = this.pledgeSector.bind(this)
    this.stop = this.stop.bind(this)

    this.connect()
  }

  async connect() {
    const token = await this.props.pondClient.call('Pond.TokenFor', [this.props.node.ID])

    this.setState(() => ({
      state: stateConnecting,
      token: token,
    }))

    const client = new Client(`ws://127.0.0.1:${this.props.node.APIPort}/rpc/v0?token=${token}`)
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

      let updates = setInterval(this.loadInfo, 1050)
      client.on('close', () => clearInterval(updates))
    })

    console.log(token) // todo: use
  }

  async loadInfo() {
    const version = await this.state.client.call("Filecoin.Version", [])
    const peers = await this.state.client.call("Filecoin.NetPeers", [])
    const actor = await this.state.client.call("Filecoin.ActorAddress", [])

    const actorState = await this.props.fullConn.call('Filecoin.StateReadState', [actor, null])

    this.setState({version: version, peers: peers.length, actor: actor, actorState: actorState})
    await this.stagedList()
  }

  async stagedList() {
    let stagedList = await this.state.client.call("Filecoin.SectorsList", [])
    let staged = await stagedList
      .map(sector => this.state.client.call("Filecoin.SectorsStatus", [sector, false]))
      .reduce(async (p, n) => [...await p, await n], Promise.resolve([]))

    let statusCounts = staged.reduce((p, n) => p.map((e, i) => e + (i === n.State ? 1 : 0) ), [0, 0, 0, 0, 0])

    this.setState({staged, statusCounts})
  }

  async pledgeSector() {
    await this.state.client.call("Filecoin.PledgeSector", [])
  }

  sealStaged = async () => {
    await this.state.client.call("Filecoin.SectorsStagedSeal", [])
  }

  async stop() {
    await this.props.stop()
  }

  render() {
    let runtime = <div></div>
    if (this.state.actor) {
      const pledgeSector = <a href="#" onClick={this.pledgeSector}>[Pledge Sector]</a>
      const sealStaged = <a href="#" onClick={this.sealStaged}>[Seal Staged]</a>

      runtime = (
        <div>
          <div>v{this.state.version.Version}, <abbr title={this.state.id}>{this.state.id.substr(-8)}</abbr>, {this.state.peers} peers</div>
          <div>Repo: LOTUS_MINER_PATH={this.props.node.Repo}</div>
          <div>
            {pledgeSector} {sealStaged}
          </div>
          <div>
            <Address client={this.props.fullConn} addr={this.state.actor} mountWindow={this.props.mountWindow}/>
            <span>&nbsp;<abbr title="Proving period end">EPS:</abbr> <b>{this.state.actorState.State.ElectionPeriodStart}</b></span>
          </div>
          <div>{this.state.statusCounts.map((c, i) => <span key={i}>{sealCodes[i]}: {c} | </span>)}</div>
          <div>
            {this.state.staged ? this.state.staged.map((s, i) => (
              <div key={i}>{s.SectorID} {sealCodes[s.State] || `unk ${s.State}`}</div>
            )) : <div/>}
          </div>

        </div>
      )
    }

    return <Window
      title={"Miner Node " + this.props.node.ID}
      initialPosition={{x: this.props.node.ID*30, y: this.props.node.ID * 30}}
      onClose={this.stop} >
      <div className="CristalScroll">
        <div className="StorageNode">
          {runtime}
        </div>
      </div>
    </Window>
  }
}

export default StorageNode
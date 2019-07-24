import React from 'react';
import { Client } from 'rpc-websockets'
import Cristal from 'react-cristal'

const stateConnected = 'connected'
const stateConnecting = 'connecting'
const stateGettingToken = 'getting-token'

class FullNode extends React.Component {
  constructor(props) {
    super(props)

    this.state = {
      state: stateGettingToken,
      id: "~"
    }

    this.loadInfo = this.loadInfo.bind(this);

    this.connect()
  }

  async connect() {
    console.log("gettok")

    const token = await this.props.pondClient.call('Pond.TokenFor', [this.props.node.ID])

    this.setState(() => ({
      state: stateConnecting,
      token: token,
    }))

    const client = new Client(`ws://127.0.0.1:${this.props.node.ApiPort}/rpc/v0?token=${token}`)
    client.on('open', () => {
      this.setState(() => ({
        state: stateConnected,
        client: client,

        version: {Version: "~version~"},
        id: "~peerid~",
        peers: -1
      }))

      this.props.onConnect(client)

      this.loadInfo()
      setInterval(this.loadInfo, 1000)
    })

    console.log(token) // todo: use
  }

  async loadInfo() {
    const version = await this.state.client.call("Filecoin.Version", [])
    this.setState(() => ({version: version}))

    const id = await this.state.client.call("Filecoin.ID", [])
    this.setState(() => ({id: id}))

    const peers = await this.state.client.call("Filecoin.NetPeers", [])
    this.setState(() => ({peers: peers.length}))

    const tipset = await this.state.client.call("Filecoin.ChainHead", [])
    this.setState(() => ({tipset: tipset}))
  }

  render() {
    let runtime = <div></div>
    if (this.state.state === stateConnected) {
      let chainInfo = <div></div>
      if (this.state.tipset !== undefined) {
        chainInfo = (
          <div>Head: {this.state.tipset.Cids.map(c => c['/'].substr(-8))} H:{this.state.tipset.Height}</div>
        )
      }

      runtime = (
        <div>
          <div>v{this.state.version.Version}, {this.state.id.substr(-8)}, {this.state.peers} peers</div>
          <div>Repo: LOTUS_PATH={this.props.node.Repo}</div>
          {chainInfo}

        </div>
      )
    }

    return (
      <Cristal
        title={"Node " + this.props.node.ID}
        initialPosition={{x: this.props.node.ID*30, y: this.props.node.ID * 30}} >

        <div className="FullNode">
          <div>{this.props.node.ID} - {this.state.state}</div>
          {runtime}
        </div>
      </Cristal>
    )
  }
}

export default FullNode
import React from 'react';
import { Client } from 'rpc-websockets'

const stateConnected = 'connected'
const stateConnecting = 'connecting'
const stateGettingToken = 'getting-token'

class FullNode extends React.Component {
  constructor(props) {
    super(props)

    this.state = {
      state: stateGettingToken
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

    const client = new Client(`ws://127.0.0.1:${this.props.node.ApiPort}/rpc/v0`)
    client.on('open', () => {
      this.setState(() => ({
        state: stateConnected,
        client: client,

        version: {Version: "~version~"},
        id: "~peerid~",
        peers: -1
      }))

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
  }

  render() {
    let runtime = <div></div>
    if (this.state.state === stateConnected) {
      runtime = (
        <div>
          <div>v{this.state.version.Version}, {this.state.id.substr(-8)}, {this.state.peers} peers</div>

        </div>
      )
    }

    return (
      <div className="FullNode">
        <div>{this.props.node.ID} - {this.state.state}</div>
        {runtime}
      </div>
    )
  }
}

export default FullNode
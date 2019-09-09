import React from 'react';
import FullNode from "./FullNode";
import ConnMgr from "./ConnMgr";
import Consensus from "./Consensus";
import {Cristal} from "react-cristal";
import StorageNode from "./StorageNode";
import {Client} from "rpc-websockets";
import pushMessage from "./chain/send";
import Logs from "./Logs";
import StorageNodeInit from "./StorageNodeInit";

class NodeList extends React.Component {
  constructor(props) {
    super(props)
    this.state = {
      existingLoaded: false,
      nodes: {},

      showConnMgr: false,
      showConsensus: false,
    }

    // This binding is necessary to make `this` work in the callback
    this.spawnNode = this.spawnNode.bind(this)
    this.spawnStorageNode = this.spawnStorageNode.bind(this)
    this.connMgr = this.connMgr.bind(this)
    this.consensus = this.consensus.bind(this)
    this.transfer1kFrom1 = this.transfer1kFrom1.bind(this)

    this.getNodes()
  }

  async mountNode(node) {
    const token = await this.props.client.call('Pond.TokenFor', [node.ID])

    const client = new Client(`ws://127.0.0.1:${node.ApiPort}/rpc/v0?token=${token}`)
    client.on('open', async () => {
      const id = await client.call("Filecoin.ID", [])

      this.setState(prev => ({
        nodes: {
          ...prev.nodes,
          [node.ID]: {...node, conn: client, peerid: id}
        }
      }))

      if (!node.Storage) {
        this.props.mountWindow((onClose) =>
            <FullNode key={node.ID}
                      node={{...node}}
                      client={client}
                      pondClient={this.props.client}
                      give1k={this.transfer1kFrom1}
                      mountWindow={this.props.mountWindow}
                      spawnStorageNode={this.spawnStorageNode}
            />)
      } else {
        const fullId = await this.props.client.call('Pond.FullID', [node.ID])

        this.props.mountWindow((onClose) =>
            <StorageNode node={{...node}}
                         pondClient={this.props.client}
                         fullConn={this.state.nodes[fullId].conn}
                         mountWindow={this.props.mountWindow}
            />)
      }
    })
  }

  async getNodes() {
    const nds = await this.props.client.call('Pond.Nodes')
    const nodes = nds.reduce((o, i) => {o[i.ID] = i; return o}, {})
    console.log('nds', nodes)

    Object.keys(nodes).map(n => nodes[n]).forEach(n => this.mountNode(n))

    this.setState({existingLoaded: true, nodes: nodes})
  }

  async transfer1kFrom1(to) {
    const addrss = await this.state.nodes[1].conn.call('Filecoin.WalletList', [])
    const [bestaddr, bal] = await addrss.map(async addr => {
      let balance = 0
      try {
        balance = await this.state.nodes[1].conn.call('Filecoin.WalletBalance', [addr])
      } catch {
        balance = -1
      }
      return [addr, balance]
    }).reduce(async (c, n) => (await c)[1] > (await n)[1] ? await c : await n, Promise.resolve(['', -2]))

    await pushMessage(this.state.nodes[1].conn, bestaddr, {
      To: to,
      From: bestaddr,
      Value: "1000",
    })
  }

  async spawnNode() {
    const node = await this.props.client.call('Pond.Spawn')
    console.log(node)
    await this.mountNode(node)

    this.setState(state => ({nodes: {...state.nodes, [node.ID]: node}}))
  }

  async spawnStorageNode(fullRepo, fullConn) {
    let nodePromise = this.props.client.call('Pond.SpawnStorage', [fullRepo])

    this.props.mountWindow((onClose) => <StorageNodeInit node={nodePromise} fullRepo={fullRepo} fullConn={fullConn} pondClient={this.props.client} onClose={onClose} mountWindow={this.props.mountWindow}/>)
    let node = await nodePromise
    await this.mountNode(node)

    //this.setState(state => ({nodes: {...state.nodes, [node.ID]: node}}))
  }

  connMgr() {
    this.setState({showConnMgr: true})
  }

  consensus() {
    this.setState({showConsensus: true})
  }

  render() {
    let connMgr
    if (this.state.showConnMgr) {
      connMgr = (<ConnMgr nodes={this.state.nodes}/>)
    }

    let consensus
    if (this.state.showConsensus) {
      consensus = (<Consensus nodes={this.state.nodes} mountWindow={this.props.mountWindow}/>)
    }

    return (
      <Cristal title={"Node List"} initialPosition="bottom-left">
        <div className={'NodeList'}>
          <div>
            <button onClick={this.spawnNode} disabled={!this.state.existingLoaded}>Spawn Node</button>
            <button onClick={this.connMgr} disabled={!this.state.existingLoaded && !this.state.showConnMgr}>Connections</button>
            <button onClick={this.consensus} disabled={!this.state.existingLoaded && !this.state.showConsensus}>Consensus</button>
          </div>
          <div>
            {Object.keys(this.state.nodes).map(n => {
              const nd = this.state.nodes[n]
              let type = "FULL"
              if (nd.Storage) {
                type = "STOR"
              }

              let logs = "[logs]"
              let info = "[CONNECTING..]"
              if (nd.conn) {
                info = <span>{nd.peerid}</span>
                logs = <a href='#' onClick={() => this.props.mountWindow(cl => <Logs node={nd.ID} onClose={cl}/>)}>[logs]</a>
              }

              return <div key={n}>
                {n} {type} {logs} {info}
              </div>
            })}
          </div>
        </div>
        <div>
          {connMgr}
          {consensus}
        </div>
      </Cristal>
    );
  }
}

export default NodeList

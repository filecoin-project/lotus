import React from 'react';
import FullNode from "./FullNode";
import ConnMgr from "./ConnMgr";
import Consensus from "./Consensus";
import {Cristal} from "react-cristal";
import StorageNode from "./StorageNode";

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
    this.connMgr = this.connMgr.bind(this)
    this.consensus = this.consensus.bind(this)

    this.getNodes()
  }

  mountNode(node) {
    if (!node.Storage) {
      this.props.mountWindow((onClose) =>
        <FullNode key={node.ID}
                  node={{...node}}
                  pondClient={this.props.client}
                  onConnect={(conn, id) => this.setState(prev => ({
                    nodes: {
                      ...prev.nodes,
                      [node.ID]: {...node, conn: conn, peerid: id}
                    }
                  }))}
                  mountWindow={this.props.mountWindow}/>)
    } else {
      this.props.mountWindow((onClose) =>
        <StorageNode node={{...node}}
                     pondClient={this.props.client}
                     mountWindow={this.props.mountWindow}/>)
    }
  }

  async getNodes() {
    const nds = await this.props.client.call('Pond.Nodes')
    const nodes = nds.reduce((o, i) => {o[i.ID] = i; return o}, {})
    console.log('nds', nodes)

    Object.keys(nodes).map(n => nodes[n]).forEach(n => this.mountNode(n))

    this.setState({existingLoaded: true, nodes: nodes})
  }

  async spawnNode() {
    const node = await this.props.client.call('Pond.Spawn')
    console.log(node)
    this.mountNode(node)

    this.setState(state => ({nodes: {...state.nodes, [node.ID]: node}}))
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
        <div>
          <button onClick={this.spawnNode} disabled={!this.state.existingLoaded}>Spawn Node</button>
          <button onClick={this.connMgr} disabled={!this.state.existingLoaded && !this.state.showConnMgr}>Connections</button>
          <button onClick={this.consensus} disabled={!this.state.existingLoaded && !this.state.showConsensus}>Consensus</button>
        </div>
        <div>
          {Object.keys(this.state.nodes).map(n => {
            return <div key={n}>
              {n}
            </div>
          })}
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

import React from 'react';
import FullNode from "./FullNode";
import ConnMgr from "./ConnMgr";

class NodeList extends React.Component {
  constructor(props) {
    super(props)
    this.state = {
      existingLoaded: false,
      nodes: [],

      showConnMgr: false,
    }

    // This binding is necessary to make `this` work in the callback
    this.spawnNode = this.spawnNode.bind(this)
    this.connMgr = this.connMgr.bind(this)

    this.props.client.call('Pond.Nodes').then(nodes => this.setState({existingLoaded: true, nodes: nodes}))
  }

  async spawnNode() {
    const node = await this.props.client.call('Pond.Spawn')
    console.log(node)
    this.setState(state => ({nodes: state.nodes.concat(node)}))
  }

  connMgr() {
    this.setState({showConnMgr: true})
  }

  render() {
    let connMgr
    if (this.state.showConnMgr) {
      connMgr = (<ConnMgr nodes={this.state.nodes}/>)
    }

    return (
      <div>
        <div>
          <button onClick={this.spawnNode} disabled={!this.state.existingLoaded}>Spawn Node</button>
          <button onClick={this.connMgr} disabled={!this.state.existingLoaded && !this.state.showConnMgr}>Connections</button>
        </div>
        <div>
          {
            this.state.nodes.map((node, i) => {
              return (<FullNode key={node.ID}
                                node={node}
                                pondClient={this.props.client}/>)
            })
          }
          {connMgr}
        </div>
      </div>
    );
  }
}

export default NodeList

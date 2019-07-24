import React from 'react';
import FullNode from "./FullNode";

class NodeList extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      existingLoaded: false,
      nodes: []
    };

    // This binding is necessary to make `this` work in the callback
    this.spawnNode = this.spawnNode.bind(this);

    this.props.client.call('Pond.Nodes').then(nodes => this.setState({existingLoaded: true, nodes: nodes}))
  }

  async spawnNode() {
    const node = await this.props.client.call('Pond.Spawn')
    console.log(node)
    this.setState(state => ({nodes: state.nodes.concat(node)}))
  }

  render() {
    return (
      <div>
        <div>
          <button onClick={this.spawnNode} disabled={!this.state.existingLoaded}>Spawn Node</button>
        </div>
        <div>
          {
            this.state.nodes.map(node => <FullNode key={node.ID} node={node} pondClient={this.props.client} />)
          }
        </div>
      </div>
    );
  }
}

export default NodeList

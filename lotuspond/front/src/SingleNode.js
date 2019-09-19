import React from 'react';
import './App.css';
import {Client} from "rpc-websockets";
import FullNode from "./FullNode";

class SingleNode extends React.Component {
  constructor(props) {
    super(props)

    const nodes = JSON.parse(window.localStorage.getItem('saved-nodes'))
    const node = nodes[this.props.match.params.node]

    const client = new Client(`${node.addr}?token=${node.token}`)
    client.on('open', async () => {
      this.setState(() => ({client: client}))
    })

    this.state = {
      windows: {},
      nextWindow: 0,

      addr: node.addr
    }
  }

  mountWindow = (cb) => {
    const id = this.state.nextWindow
    this.setState({nextWindow: id + 1})

    const window = cb(() => {
      this.setState(prev => ({windows: {...prev.windows, [id]: undefined}}))
    })

    this.setState(prev => ({windows: {...prev.windows, [id]: window}}))
  }

  render() {
    if (this.state.client === undefined) {
      return (
        <div className="SingleNode-connecting">
          <div>
            <div>Connecting to Node RPC:</div>
            <div>{`${this.state.addr}?token=****`}</div>
          </div>
        </div>
      )
    }

    let node = <FullNode
      node={{Repo: '/i/dont/exist/fixme', ID: ''}}
      client={this.state.client}
      give1k={null}
      mountWindow={this.mountWindow}
    />

    return (
      <div className="App">
        {node}
        <div>
          {Object.keys(this.state.windows).map((w, i) => <div key={i}>{this.state.windows[w]}</div>)}
        </div>
      </div>
    )
  }
}

export default SingleNode;

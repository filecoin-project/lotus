import React from 'react';
import './App.css';
import { Client } from 'rpc-websockets'
import NodeList from "./NodeList";


class App extends React.Component {
  constructor(props) {
    super(props)

    const client = new Client('ws://127.0.0.1:2222/rpc/v0')
    client.on('open', () => {
      this.setState(() => ({client: client}))
    })

    this.state = {}
  }

  render() {
    if (this.state.client === undefined) {
      return (
          <div>
            Connecting to RPC
          </div>
      )
    }

    return (
        <div className="App">
          <NodeList client={this.state.client}/>
        </div>
    )
  }
}

export default App

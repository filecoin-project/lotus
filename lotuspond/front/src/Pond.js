import React from 'react';
import './App.css';
import { Client } from 'rpc-websockets'
import NodeList from "./NodeList";


class Pond extends React.Component {
  constructor(props) {
    super(props)

    const client = new Client('ws://127.0.0.1:2222/rpc/v0')
    client.on('open', () => {
      this.setState(() => ({client: client}))
    })

    this.state = {
      windows: {},
      nextWindow: 0,
    }

    this.mountWindow = this.mountWindow.bind(this)
  }

  mountWindow(cb) {
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
        <div className="Pond-connecting">
          <div>
            <div>Connecting to Pond RPC</div>
          </div>
        </div>
      )
    }

    return (
        <div className="App">
          <NodeList client={this.state.client} mountWindow={this.mountWindow}/>
          <div>
            {Object.keys(this.state.windows).map((w, i) => <div key={i}>{this.state.windows[w]}</div>)}
          </div>
        </div>
    )
  }
}

export default Pond

import React from 'react'
import { Terminal } from 'xterm';
import { AttachAddon } from "xterm-addon-attach";
import 'xterm/dist/xterm.css';
import * as fit from 'xterm/lib/addons/fit/fit';
import Window from "./Window";

class Logs extends React.Component {
  constructor(props) {
    super(props);
    this.termRef = React.createRef()
    this.winRef = React.createRef()
  }

  async componentDidMount() {
    Terminal.applyAddon(fit);

    this.terminal = new Terminal({convertEol: true, fontSize: 11});
    this.terminal.loadAddon(new AttachAddon(new WebSocket(`ws://127.0.0.1:2222/logs/${this.props.node}`), {bidirectional: false, inputUtf8: true}))
    this.terminal.open(this.termRef.current)
    setInterval(() => this.terminal.fit(), 200)
  }

  render() {
    return <Window className="Logs-window" onClose={this.props.onClose} initialSize={{width: 1000, height: 480}} title={`Node ${this.props.node} Logs`}>
      <div ref={this.termRef} className="Logs"/>
    </Window>
  }
}

export default Logs

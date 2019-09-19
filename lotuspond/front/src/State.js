import React from 'react'
import Window from "./Window";

class State extends React.Component {
  constructor(props) {
    super(props)

    this.state = {Balance: -2, State: {}}
  }

  async componentDidMount() {
    const tipset = await this.props.client.call("Filecoin.ChainHead", []) // TODO: from props
    const actstate = await this.props.client.call('Filecoin.StateReadState', [this.props.actor, tipset])
    this.setState(actstate)
  }

  render() {
    const content = <div className="State">
      <div>Balance: {this.state.Balance}</div>
      <div>---</div>
      <div>{Object.keys(this.state.State).map(k => <div key={k}>{k}: <span>{JSON.stringify(this.state.State[k])}</span></div>)}</div>
    </div>

    return <Window onClose={this.props.onClose} title={`Actor ${this.props.addr} @{this.props.ts.Height}`}>
      {content}
    </Window>
  }
}

export default State
import React from 'react'
import {Cristal} from "react-cristal";

class State extends React.Component {
  constructor(props) {
    super(props)

    this.state = {Balance: -2, State: {}}
  }

  async componentDidMount() {
    const tipset = await this.props.client.call("Filecoin.ChainHead", []) // TODO: from props
    const actstate = await this.props.client.call('Filecoin.ChainReadState', [this.props.actor, tipset])
    this.setState(actstate)
  }

  render() {
    const content = <div className="State">
      <div>Balance: {this.state.Balance}</div>
      <div>---</div>
      <div>{Object.keys(this.state.State).map(k => <div key={k}>{k}: <span>{JSON.stringify(this.state.State[k])}</span></div>)}</div>
    </div>

    return <Cristal onClose={this.props.onClose} title={`Actor ${this.props.addr} @{this.props.ts.Height}`}>
      {content}
    </Cristal>
  }
}

export default State
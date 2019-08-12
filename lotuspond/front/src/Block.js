import React from 'react';
import {Cristal} from "react-cristal";
import {BlockLinks} from "./BlockLink";
import Address from "./Address";

class Block extends React.Component {
  constructor(props) {
    super(props)

    this.state = {}

    this.loadHeader()
  }

  async loadHeader() {
    const header = await this.props.conn.call('Filecoin.ChainGetBlock', [this.props.cid])
    const messages = await this.props.conn.call('Filecoin.ChainGetBlockMessages', [this.props.cid])
    console.log(messages)
    this.setState({header: header, messages: messages})
  }

  render() {
    let content = <div>Loading Block Info</div>
    if (this.state.header) {
      let head = this.state.header



      let messages = [
        ...(this.state.messages.BlsMessages.map(m => ({...m, type: 'BLS'}))),
        ...(this.state.messages.SecpkMessages.map(m => ({...(m.Message), type: 'Secpk'})))
      ].map(m => (
        <div>
          <Address client={this.props.conn} addr={m.From} mountWindow={this.props.mountWindow}/><b>=>&nbsp;</b>
          <Address client={this.props.conn} addr={m.To} mountWindow={this.props.mountWindow}/>
          {m.Value}FIL&nbsp;M{m.Method}
        </div>
      ))

      content = (
        <div className="Block">
          <div>Height: {head.Height}</div>
          <div>Parents: <BlockLinks cids={head.Parents} conn={this.props.conn} mountWindow={this.props.mountWindow}/></div>
          <div>Weight: {head.ParentWeight}</div>
          <div>Miner: {<Address client={this.props.conn} addr={head.Miner} mountWindow={this.props.mountWindow}/>}</div>
          <div>Messages: {head.Messages['/']} {/*TODO: link to message explorer */}</div>
          <div>Receipts: {head.MessageReceipts['/']}</div>
          <div>State Root:&nbsp;{head.StateRoot['/']}</div>
          <div>----</div>
          <div>{messages}</div>
        </div>
      )
    }

    return (<Cristal onClose={this.props.onClose} title={`Block ${this.props.cid['/']}`}>
      {content}
    </Cristal>)
  }
}

export default Block
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
    let messages = await this.props.conn.call('Filecoin.ChainGetBlockMessages', [this.props.cid])
    let receipts = await this.props.conn.call('Filecoin.ChainGetBlockReceipts', [this.props.cid])

    messages = [
      ...(messages.BlsMessages.map(m => ({...m, type: 'BLS'}))),
      ...(messages.SecpkMessages.map(m => ({...(m.Message), type: 'Secpk'})))
    ]

    messages = messages.map((msg, k) => ({...msg, receipt: receipts[k]}))

    this.setState({header: header, messages: messages})
  }

  render() {
    let content = <div>Loading Block Info</div>
    if (this.state.header) {
      let head = this.state.header

      const messages = this.state.messages.map(m => (
        <div>
          <Address client={this.props.conn} addr={m.From} mountWindow={this.props.mountWindow}/><b>&nbsp;=>&nbsp;</b>
          <Address client={this.props.conn} addr={m.To} mountWindow={this.props.mountWindow} transfer={m.Value} method={m.Method}/>
          <span>&nbsp;N{m.Nonce}</span>
          <span>&nbsp;{m.receipt.GasUsed}Gas</span>
          {m.receipt.ExitCode !== 0 ? <span>&nbsp;<b>EXIT:{m.receipt.ExitCode}</b></span> : <span/>}
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

    return (<Cristal className="CristalScroll" initialSize={{width: 700, height: 400}} onClose={this.props.onClose} title={`Block ${this.props.cid['/']}`}>
      {content}
    </Cristal>)
  }
}

export default Block
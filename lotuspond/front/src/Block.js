import React from 'react';
import {BlockLinks} from "./BlockLink";
import Address from "./Address";
import Window from "./Window";

class Block extends React.Component {
  constructor(props) {
    super(props)

    this.state = {}

    this.loadHeader()
  }

  async loadHeader() {
    const header = await this.props.conn.call('Filecoin.ChainGetBlock', [this.props.cid])
    let messages = await this.props.conn.call('Filecoin.ChainGetParentMessages', [this.props.cid])
    let receipts = await this.props.conn.call('Filecoin.ChainGetParentReceipts', [this.props.cid])

    if (!messages) {
      messages = []
    }


    messages = messages.map((msg, k) => ({...msg.Message, cid: msg.Cid, receipt: receipts[k]}))

    messages = await Promise.all(messages.map(async (msg, i) => {
      if (msg.receipt.ExitCode !== 0) {
        let reply = await this.props.conn.call('Filecoin.StateReplay', [{Cids: [this.props.cid], Blocks: [header], Height: header.Height}, msg.Cid])
        if(!reply.Error) {
          reply.Error = "reply: no error"
        }
        msg.Error = reply.Error
      }
      return msg
    }))

    this.setState({header: header, messages: messages})
  }

  render() {
    let content = <div>Loading Block Info</div>
    if (this.state.header) {
      let head = this.state.header

      const messages = this.state.messages.map((m, k) => (
        <div key={k}>
          <div>
            <Address client={this.props.conn} addr={m.From} mountWindow={this.props.mountWindow}/><b>&nbsp;=>&nbsp;</b>
            <Address client={this.props.conn} addr={m.To} mountWindow={this.props.mountWindow} transfer={m.Value} method={m.Method}/>
            <span>&nbsp;N{m.Nonce}</span>
            <span>&nbsp;{m.receipt.GasUsed}Gas</span>
            {m.receipt.ExitCode !== 0 ? <span>&nbsp;<b>EXIT:{m.receipt.ExitCode}</b></span> : <span/>}
          </div>
          {m.receipt.ExitCode !== 0 ? <div>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;Error: <b>{m.Error}</b></div> : <span/>}
        </div>
      ))

      content = (
        <div className="Block">
          <div>Height: {head.Height}</div>
          <div>Parents: <BlockLinks cids={head.Parents} conn={this.props.conn} mountWindow={this.props.mountWindow}/></div>
          <div>Weight: {head.ParentWeight}</div>
          <div>Miner: {<Address client={this.props.conn} addr={head.Miner} mountWindow={this.props.mountWindow}/>}</div>
          <div>Messages: {head.Messages['/']} {/*TODO: link to message explorer */}</div>
          <div>Parent Receipts: {head.ParentMessageReceipts['/']}</div>
          <div>
            <span>Parent State Root:&nbsp;{head.ParentStateRoot['/']}</span>
            <span>&nbsp;<Address client={this.props.conn} short={true} addr="t00" mountWindow={this.props.mountWindow}/></span>
            <span>&nbsp;<Address client={this.props.conn} short={true} addr="t01" mountWindow={this.props.mountWindow}/></span>
            <span>&nbsp;<Address client={this.props.conn} short={true} addr="t02" mountWindow={this.props.mountWindow}/></span>
            <span>&nbsp;<Address client={this.props.conn} short={true} addr="t03" mountWindow={this.props.mountWindow}/></span>
            <span>&nbsp;<Address client={this.props.conn} short={true} addr="t04" mountWindow={this.props.mountWindow}/></span>
            <span>&nbsp;<Address client={this.props.conn} short={true} addr="t05" mountWindow={this.props.mountWindow}/></span>
            <span>&nbsp;<Address client={this.props.conn} short={true} addr="t099" mountWindow={this.props.mountWindow}/></span>
          </div>
          <div>----</div>
          <div>{messages}</div>
        </div>
      )
    }

    return (<Window className="CristalScroll" initialSize={{width: 1050, height: 400}} onClose={this.props.onClose} title={`Block ${this.props.cid['/']}`}>
      {content}
    </Window>)
  }
}

export default Block
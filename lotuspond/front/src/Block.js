import React from 'react';
import {Cristal} from "react-cristal";
import {BlockLinks} from "./BlockLink";

class Block extends React.Component {
  constructor(props) {
    super(props)

    this.state = {}

    this.loadHeader()
  }

  async loadHeader() {
    const header = await this.props.conn.call('Filecoin.ChainGetBlock', [this.props.cid])
    this.setState({header: header})
  }

  render() {
    let content = <div>Loading Block Info</div>
    if (this.state.header) {
      let head = this.state.header

      content = (
        <div>
          <div>Height: {head.Height}</div>
          <div>Parents: <BlockLinks cids={head.Parents} conn={this.props.conn} mountWindow={this.props.mountWindow}/></div>
          <div>Weight: {head.ParentWeight}</div>
          <div>Miner: {head.Miner}</div>
          <div>Messages: {head.Messages['/']} {/*TODO: link to message explorer */}</div>
          <div>Receipts: {head.MessageReceipts['/']}</div>
          <div>State Root: {head.StateRoot['/']}</div>


        </div>
      )
    }

    return (<Cristal onClose={this.props.onClose} title={`Block ${this.props.cid['/']}`}>
      {content}
    </Cristal>)
  }
}

export default Block
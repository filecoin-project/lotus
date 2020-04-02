import React from 'react';
import Block from "./Block";
import Address from "./Address";


export class BlockLinks extends React.Component {
  render() {
    return this.props.cids.map((c, k) => {
      let block

      if(this.props.blocks) {
        block = this.props.blocks[k]
      }

      return <span key={c + '-' + k}><BlockLink block={block} conn={this.props.conn} cid={c} mountWindow={this.props.mountWindow}/> </span>
    })
  }
}

class BlockLink extends React.Component {
  constructor(props) {
    super(props)

    this.openBlockViewer = this.openBlockViewer.bind(this)
  }

  openBlockViewer() {
    this.props.mountWindow((onClose) => <Block cid={this.props.cid} conn={this.props.conn} onClose={onClose} mountWindow={this.props.mountWindow}/>)
  }

  render() {
    let info = <span></span>
    if(this.props.block) {
      info = <span>&nbsp;(by <Address client={this.props.conn} addr={this.props.block.Miner} mountWindow={this.props.mountWindow} short={true}/>)</span>
    }

    return <span><a href="#" onClick={this.openBlockViewer}><abbr title={this.props.cid['/']}>{this.props.cid['/'].substr(-8)}</abbr></a>{info}</span>
  }
}

export default BlockLink
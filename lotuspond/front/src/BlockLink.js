import React from 'react';
import Block from "./Block";


export class BlockLinks extends React.Component {
  render() {
    return this.props.cids.map(c => <BlockLink key={c} conn={this.props.conn} cid={c} mountWindow={this.props.mountWindow}/>)
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
    return <a href="#" onClick={this.openBlockViewer}><abbr title={this.props.cid['/']}>{this.props.cid['/'].substr(-8)}</abbr></a>
  }
}

export default BlockLink
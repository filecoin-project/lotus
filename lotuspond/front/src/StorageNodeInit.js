import React from 'react';
import {Cristal} from "react-cristal";
import StorageNode from "./StorageNode";

class StorageNodeInit extends React.Component {
  async componentDidMount() {
    const info = await this.props.node

    this.props.onClose()
    //this.props.mountWindow((onClose) => <StorageNode node={info} fullRepo={this.props.fullRepo} fullConn={this.props.fullConn} pondClient={this.props.pondClient} onClose={onClose} mountWindow={this.props.mountWindow}/>)
  }

  render() {
    return <Cristal
      title={"Storage miner initializing"}
      initialPosition={'center'}>
      <div className="CristalScroll">
        <div className="StorageNodeInit">
          Waiting for init, make sure at least one miner is enabled
        </div>
      </div>
    </Cristal>
  }
}

export default StorageNodeInit
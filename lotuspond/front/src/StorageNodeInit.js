import React from 'react';
import Window from "./Window";

class StorageNodeInit extends React.Component {
  async componentDidMount() {
    const info = await this.props.node

    this.props.onClose()
    //this.props.mountWindow((onClose) => <StorageNode node={info} fullRepo={this.props.fullRepo} fullConn={this.props.fullConn} pondClient={this.props.pondClient} onClose={onClose} mountWindow={this.props.mountWindow}/>)
  }

  render() {
    return <Window
      title={"Miner initializing"}
      initialPosition={'center'}>
      <div className="CristalScroll">
        <div className="StorageNodeInit">
          Waiting for init, make sure at least one miner is enabled
        </div>
      </div>
    </Window>
  }
}

export default StorageNodeInit
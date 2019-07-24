import React from 'react';
import Cristal from 'react-cristal'

class ConnMgr extends React.Component {
  render() {
    return(
      <Cristal title="Connection Manager">
        {Object.keys(this.props.nodes)}
      </Cristal>
    )
  }
}

export default ConnMgr
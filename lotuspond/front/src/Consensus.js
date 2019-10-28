import React from 'react';
import {BlockLinks} from "./BlockLink";
import Window from "./Window";

function styleForHDiff(max, act) {
  switch (max - act) {
    case 0:
      return {background: '#004400'}
    case 1:
      return {background: '#aaaa00'}
    default:
      return {background: '#aa0000'}
  }
}

class Consensus extends React.Component {
  constructor(props) {
    super(props)

    this.state = {
      maxH: -1,
      tipsets: []
    }

    this.updateNodes = this.updateNodes.bind(this)

    setInterval(this.updateNodes, 333)
  }

  async updateNodes() {
    const nodes = this.props.nodes
    let keys = Object.keys(nodes).filter(k => !nodes[k].Storage)

    const tipsets = await keys.map(async k => {
      const tipset = await nodes[k].conn.call("Filecoin.ChainHead", [])
      return [k, tipset]
    }).reduce(async(p, i) => ([...await p, await i]), Promise.resolve([]))

    const maxH = tipsets.reduce((p, [_, i]) => Math.max(p, i.Height), -1)

    this.setState({maxH, tipsets})
  }

  render() {
    return (<Window title={`Consensus`}>
      <div className='Consensus'>
        <div>Max Height: {this.state.maxH}</div>
        <div>
          <table cellSpacing={0}>
            <thead>
              <tr><td>Node</td><td>Height</td><td>TipSet</td></tr>
            </thead>
            <tbody>
              {this.state.tipsets.map(([k, ts]) => {
                return (
                  <tr style={styleForHDiff(this.state.maxH, ts.Height)}>
                    <td>{k}</td><td>{ts.Height}</td><td><BlockLinks cids={ts.Cids} conn={this.props.nodes[k].conn} mountWindow={this.props.mountWindow}/></td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>
    </Window>)
  }
}

export default Consensus
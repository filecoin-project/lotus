import React from 'react';
import {Cristal} from "react-cristal";

function styleForHDiff(max, act) {
  switch (max - act) {
    case 0:
      return {background: '#00aa00'}
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
    let keys = Object.keys(nodes)

    const tipsets = await keys.map(async k => {
      const tipset = await nodes[k].conn.call("Filecoin.ChainHead", [])
      return [k, tipset]
    }).reduce(async(p, i) => ([...await p, await i]), Promise.resolve([]))

    const maxH = tipsets.reduce((p, [_, i]) => Math.max(p, i.Height), -1)

    this.setState({maxH, tipsets})
  }

  render() {
    return (<Cristal title={`Consensus`}>
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
                    <td>{k}</td><td>{ts.Height}</td><td>{ts.Cids.map(c => <abbr title={c['/']}>{c['/'].substr(-8)}</abbr>)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>
    </Cristal>)
  }
}

export default Consensus
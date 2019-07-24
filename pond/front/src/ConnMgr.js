import React from 'react';
import Cristal from 'react-cristal'

class ConnMgr extends React.Component {
  constructor(props) {
    super(props)

    this.connect = this.connect.bind(this)
  }

  async connect(action, from, to) {
    if (action) {
      const fromNd = this.props.nodes[from]
      const toNd = this.props.nodes[to]

      const toPeerInfo = await toNd.conn.call('Filecoin.NetAddrsListen', [])

      console.log("conn")
      await fromNd.conn.call('Filecoin.NetConnect', [toPeerInfo])
    }
  }

  render() {
    const nodes = this.props.nodes
    let keys = Object.keys(nodes)

    const rows = keys.filter((_, i) => i > 0).map((k, i) => {
      const cols = keys.filter((_, j) => i >= j).map((kt, i) => {
        return (<td><input key={k + "-" + kt} type="checkbox" onChange={e => this.connect(e.target.checked, k, kt)}/></td>)
      })
      return (
          <tr><td>{k}</td>{cols}</tr>
      )
    })

    return(
      <Cristal title="Connection Manager">
        <table>
          <thead><tr><td></td>{keys.slice(0, -1).map((i) => (<td>{i}</td>))}</tr></thead>
          <tbody>{rows}</tbody>
        </table>
      </Cristal>
    )
  }
}

export default ConnMgr
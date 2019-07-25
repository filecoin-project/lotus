import React from 'react';
import Cristal from 'react-cristal'

class ConnMgr extends React.Component {
  constructor(props) {
    super(props)

    this.connect = this.connect.bind(this)
    this.connectAll = this.connectAll.bind(this)
    this.connect1 = this.connect1.bind(this)
    this.connectChain = this.connectChain.bind(this)

    this.state = {conns: {}}
  }

  async connect(action, from, to) {
    if (action) {
      const fromNd = this.props.nodes[from]
      const toNd = this.props.nodes[to]

      const toPeerInfo = await toNd.conn.call('Filecoin.NetAddrsListen', [])

      await fromNd.conn.call('Filecoin.NetConnect', [toPeerInfo])
    }

    this.setState(prev => ({conns: {...prev.conns, [`${from},${to}`]: action}}))
  }

  connectAll() {
    const nodes = this.props.nodes
    let keys = Object.keys(nodes)

    keys.filter((_, i) => i > 0).forEach((k, i) => {
      keys.filter((_, j) => i >= j).forEach((kt, i) => {
        this.connect(true, k, kt)
      })
    })
  }

  connect1() {
    const nodes = this.props.nodes
    let keys = Object.keys(nodes)

    keys.filter((_, i) => i > 0).forEach((k, i) => {
      this.connect(true, k, keys[0])
    })
  }

  connectChain() {
    const nodes = this.props.nodes
    let keys = Object.keys(nodes)

    keys.filter((_, i) => i > 0).forEach((k, i) => {
      this.connect(true, k, keys[i])
    })
  }

  render() {
    const nodes = this.props.nodes
    let keys = Object.keys(nodes)

    const rows = keys.filter((_, i) => i > 0).map((k, i) => {
      const cols = keys.filter((_, j) => i >= j).map((kt, i) => {
        const checked = this.state.conns[`${k},${kt}`] === true

        return (<td key={k + "," + kt}><input checked={checked} type="checkbox" onChange={e => this.connect(e.target.checked, k, kt)}/></td>)
      })
      return (
          <tr key={k}><td>{k}</td>{cols}</tr>
      )
    })

    return(
      <Cristal title="Connection Manager">
        <table>
          <thead><tr><td></td>{keys.slice(0, -1).map((i) => (<td key={i}>{i}</td>))}</tr></thead>
          <tbody>{rows}</tbody>
        </table>
        <div>
          <button onClick={this.connectAll}>ConnAll</button>
          <button onClick={this.connect1}>Conn1</button>
          <button onClick={this.connectChain}>ConnChain</button>
        </div>
      </Cristal>
    )
  }
}

export default ConnMgr
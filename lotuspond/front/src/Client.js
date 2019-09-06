import React from 'react';
import Cristal from 'react-cristal'

class Client extends React.Component {
  constructor(props) {
    super(props)

    this.state = {
      kbs: 1,
      blocks: 12,
      total: 36000,
      miner: "t0101"
    }
  }

  update = (name) => (e) => this.setState({ [name]: e.target.value });

  makeDeal = async () => {
    let file = await this.props.pondClient.call('Pond.CreateRandomFile', [this.state.kbs * 1000]) // 1024 won't fit in 1k blocks :(
    let cid = await this.props.client.call('Filecoin.ClientImport', [file])
    let dealcid = await this.props.client.call('Filecoin.ClientStartDeal', [cid, this.state.miner, `${Math.round(this.state.total / this.state.blocks)}`, this.state.blocks])
    console.log("deal cid: ", dealcid)
  }

  render() {
    let ppb = Math.round(this.state.total / this.state.blocks * 100) / 100
    let ppmbb = Math.round(ppb / (this.state.kbs / 1000) * 100) / 100

    let dealMaker = <div>
      <span>Make Deal: </span>
      <select><option>t0101</option></select>
      <abbr title="Data length">L:</abbr> <input placeholder="KBs" defaultValue={1} onChange={this.update("kbs")}/>
      <abbr title="Deal duration">Dur:</abbr><input placeholder="blocks" defaultValue={12} onChange={this.update("blocks")}/>
      Total: <input placeholder="total price" defaultValue={36000} onChange={this.update("total")}/>
      <span><abbr title="Price per block">PpB:</abbr> {ppb} </span>
      <span><abbr title="Price per block-MiB">PpMbB:</abbr> {ppmbb} </span>
      <button onClick={this.makeDeal}>Deal!</button>
    </div>

    return <Cristal title={"Client - Node " + this.props.node.ID}>
      <div>{dealMaker}</div>
    </Cristal>
  }
}

export default Client

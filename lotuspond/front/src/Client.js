import React from 'react';
import Cristal from 'react-cristal'
import Address from "./Address";

const dealStates = [
  "Unknown",
  "Rejected",
  "Accepted",
  "Started",
  "Failed",
  "Staged",
  "Sealing",
  "Complete",
  "Error",
  "Expired"
]


class Client extends React.Component {
  constructor(props) {
    super(props)

    this.state = {
      kbs: 1,
      blocks: 12,
      total: 36000,
      miner: "t0101",

      deals: []
    }
  }

  componentDidMount() {
    this.getDeals()
    setInterval(this.getDeals, 1325)
  }

  getDeals = async () => {
    let deals = await this.props.client.call('Filecoin.ClientListDeals', [])
    this.setState({deals})
  }

  update = (name) => (e) => this.setState({ [name]: e.target.value });

  makeDeal = async () => {
    let file = await this.props.pondClient.call('Pond.CreateRandomFile', [this.state.kbs * 1000]) // 1024 won't fit in 1k blocks :(
    let cid = await this.props.client.call('Filecoin.ClientImport', [file])
    let dealcid = await this.props.client.call('Filecoin.ClientStartDeal', [cid, this.state.miner, `${Math.round(this.state.total / this.state.blocks)}`, Number(this.state.blocks)])
    console.log("deal cid: ", dealcid)
  }

  retrieve = (deal) => async () => {
    console.log(deal)
    let client = await this.props.client.call('Filecoin.WalletDefaultAddress', [])

    let order = {
      Root: deal.PieceRef,
      Size: deal.Size,
      // TODO: support offset
      Total: "900",

      Client: client,
      Miner: deal.Miner
    }

    await this.props.client.call('Filecoin.ClientRetrieve', [order, '/dev/null'])
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

    let deals = this.state.deals.map((deal, i) => <div key={i}>
      <ul>
        <li>{i}. Proposal: {deal.ProposalCid['/'].substr(0, 18)}... <Address nobalance={true} client={this.props.client} addr={deal.Miner} mountWindow={this.props.mountWindow}/>: <b>{dealStates[deal.State]}</b>
          {dealStates[deal.State] === 'Complete' ? <span>&nbsp;<a href="#" onClick={this.retrieve(deal)}>[Retrieve]</a></span> : <span/> }
          <ul>
            <li>Data: {deal.PieceRef['/']}, <b>{deal.Size}</b>B; Duration: <b>{deal.Duration}</b>Blocks</li>
            <li>Total: <b>{deal.TotalPrice}</b>FIL; Per Block: <b>{Math.round(deal.TotalPrice / deal.Duration * 100) / 100}</b>FIL; PerMbyteByteBlock: <b>{Math.round(deal.TotalPrice / deal.Duration / (deal.Size / 1000000) * 100) / 100}</b>FIL</li>
          </ul>
        </li>
      </ul>

    </div>)

    return <Cristal title={"Client - Node " + this.props.node.ID}>
      <div className="Client">
        <div>{dealMaker}</div>
        <div>{deals}</div>
      </div>
    </Cristal>
  }
}

export default Client

import React from 'react'
import Address from './Address'
import Window from './Window'
import Fil from './Fil'

const dealStates = [
  'Unknown',
  'ProposalNotFound',
  'ProposalRejected',
  'ProposalAccepted',
  'Staged',
  'Sealing',
  'ProposalSigned',
  'Published',
  'Committed',
  'Active',
  'Failing',
  'Recovering',
  'Expired',
  'NotFound',

  'Validating',
  'Transferring',
  'VerifyData',
  'Publishing',
  'Error'
]

class Client extends React.Component {
  constructor(props) {
    super(props)

    this.state = {
      miners: ['t0101'],
      ask: { Price: '1000000000' }, // 2x min default ask to account for bin packing (could also do the math correctly below, but..)

      kbs: 1,
      blocks: 12,
      total: 36000,
      miner: 't0101',

      deals: [],

      blockDelay: 10
    }
  }

  async componentDidMount() {
    let ver = await this.props.client.call('Filecoin.Version', [])
    this.setState({ blockDelay: ver.BlockDelay })

    this.getDeals()
    setInterval(this.getDeals, 1325)
  }

  getDeals = async () => {
    let miners = await this.props.client.call('Filecoin.StateListMiners', [
      null
    ])
    let deals = await this.props.client.call('Filecoin.ClientListDeals', [])
    miners.sort()
    this.setState({ deals, miners })
  }

  update = name => e => this.setState({ [name]: e.target.value })

  makeDeal = async () => {
    let perBlk =
      ((this.state.ask.Price * this.state.kbs * 1000) / (1 << 30)) * 2

    let file = await this.props.pondClient.call('Pond.CreateRandomFile', [
      this.state.kbs * 1000
    ]) // 1024 won't fit in 1k blocks :(
    let cid = await this.props.client.call('Filecoin.ClientImport', [
      {
        Path: file,
        IsCar: false
      }
    ])
    let dealcid = await this.props.client.call('Filecoin.ClientStartDeal', [
      cid,
      this.state.miner,
      `${Math.round(perBlk)}`,
      Number(this.state.blocks)
    ])
    console.log('deal cid: ', dealcid)
  }

  retrieve = deal => async () => {
    console.log(deal)
    let client = await this.props.client.call(
      'Filecoin.WalletDefaultAddress',
      []
    )

    let order = {
      Root: deal.PieceRef,
      Size: deal.Size,
      // TODO: support offset
      Total: String(deal.Size * 2),

      Client: client,
      Miner: deal.Miner
    }

    await this.props.client.call('Filecoin.ClientRetrieve', [
      order,
      {
        Path: '/dev/null',
        IsCAR: false
      }
    ])
  }

  render() {
    let perBlk = this.state.ask.Price * this.state.kbs * 1000
    let total = perBlk * this.state.blocks
    let days = (this.state.blocks * this.state.blockDelay) / 60 / 60 / 24

    let dealMaker = (
      <div hidden={!this.props.pondClient}>
        <div>
          <span>Make Deal: </span>
          <select>
            {this.state.miners.map(m => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
          <span>
            {' '}
            Ask:{' '}
            <b>
              <Fil>{this.state.ask.Price}</Fil>
            </b>{' '}
            Fil/Byte/Block
          </span>
        </div>
        <div>
          Data Size:{' '}
          <input
            type="text"
            placeholder="KBs"
            defaultValue={1}
            onChange={this.update('kbs')}
            style={{ width: '5em' }}
          />
          KB; Duration:
          <input
            type="text"
            placeholder="blocks"
            defaultValue={12}
            onChange={this.update('blocks')}
            style={{ width: '5em' }}
          />
          Blocks
        </div>
        <div>
          Total: <Fil>{total}</Fil>; {days} Days
        </div>
        <button onClick={this.makeDeal}>Deal!</button>
      </div>
    )

    let deals = this.state.deals.map((deal, i) => (
      <div key={i}>
        <ul>
          <li>
            {i}. Proposal: {deal.ProposalCid['/'].substr(0, 18)}...{' '}
            <Address
              nobalance={true}
              client={this.props.client}
              addr={deal.Provider}
              mountWindow={this.props.mountWindow}
            />
            : <b>{dealStates[deal.State]}</b>
            {dealStates[deal.State] === 'Complete' ? (
              <span>
                &nbsp;
                <a href="#" onClick={this.retrieve(deal)}>
                  [Retrieve]
                </a>
              </span>
            ) : (
              <span />
            )}
            <ul>
              <li>
                Data: {deal.PieceRef['/']}, <b>{deal.Size}</b>B; Duration:{' '}
                <b>{deal.Duration}</b>Blocks
              </li>
              <li>
                Total: <b>{deal.TotalPrice}</b>FIL; Per Block:{' '}
                <b>
                  {Math.round((deal.TotalPrice / deal.Duration) * 100) / 100}
                </b>
                FIL; PerMbyteByteBlock:{' '}
                <b>
                  {Math.round(
                    (deal.TotalPrice / deal.Duration / (deal.Size / 1000000)) *
                      100
                  ) / 100}
                </b>
                FIL
              </li>
            </ul>
          </li>
        </ul>
      </div>
    ))

    return (
      <Window
        title={'Client - Node ' + this.props.node.ID}
        onClose={this.props.onClose}
        initialSize={{ width: 600, height: 400 }}
      >
        <div className="Client">
          <div>{dealMaker}</div>
          <div>{deals}</div>
        </div>
      </Window>
    )
  }
}

export default Client

import React from 'react';
import {BlockLinks} from "./BlockLink";
import Window from "./Window";

const rows = 32

class ChainExplorer extends React.Component {
  constructor(props) {
    super(props)

    this.update = this.update.bind(this)
    this.scroll = this.scroll.bind(this)

    this.state = {
      follow: true,
      at: props.ts.Height,
      highest: props.ts,

      cache: {[props.ts.Height]: props.ts},
      messages: {},
    }
  }

  viewport() {
    const base = this.state.at - this.state.at % rows

    return Array(rows).fill(0)
      .map((_, k) => k + base)
      .map(k => k > this.state.at ? k - rows : k)
  }

  async componentDidMount() {
    let msgcache = {}
    await this.updateMessages(this.props.ts.Cids, msgcache)
    this.setState(prev => ({messages: {...prev.messages, ...msgcache}}))

    setInterval(this.update, 1000)
  }

  async updateMessages(cids, msgcache) {
    const msgs = await Promise.all(cids.map(async cid => [cid['/'], await this.props.client.call('Filecoin.ChainGetParentMessages', [cid])]))
    msgs.forEach(([cid, msg]) => msgcache[cid] = msg)
  }

  async update() {
    const tipset = await this.props.client.call("Filecoin.ChainHead", [])
    if(tipset.Height > this.state.highest.Height) {
      let msgcache = {}
      await this.updateMessages(tipset.Cids, msgcache)

      this.setState(prev => ({highest: tipset, messages: {...prev.messages, ...msgcache}, cache: {...prev.cache, [tipset.Height]: tipset}}))
      if(this.state.follow) {
        this.setState({at: tipset.Height})
      }
    }

    await this.fetchVisible()
  }

  async fetch(h, cache, msgcache) {
    if (h < 0) {
      return
    }
    const cids = cache[h + 1].Blocks.map(b => b.Parents).reduce((acc, val) => acc.concat(val), [])
    const blocks = await Promise.all(cids.map(cid => this.props.client.call('Filecoin.ChainGetBlock', [cid])))

    cache[h] = {
      Height: h,
      Cids: cids,
      Blocks: blocks,
    }

    await this.updateMessages(cids, msgcache)
  }

  async fetchVisible() {
    await this.fetchN(this.state.at)
  }

  async fetchN(top) {
    if(!this.state.cache[top]) {
      if(top === this.state.highest.Height) {
        throw "fetchN broke (tipset not fetched)"
      }
      let h = top + rows > this.state.highest.Height ? this.state.highest.Height : top + rows

      await this.fetchN(h)
    }

    const tofetch = Array(rows).fill(0).map((_, i) => top - i)
      .filter(v => !this.state.cache[v])

    let cache = {...this.state.cache}
    let msgcache = {...this.state.messages}

    await tofetch.reduce(async (prev, next) => [...await prev, await this.fetch(next, cache, msgcache)], Promise.resolve([]))
    this.setState({cache: cache, messages: msgcache})
  }

  scroll(event) {
    if(event.deltaY < 0 && this.state.at > 0) {
      this.setState(prev => ({at: prev.at - 1, follow: false}))
    }
    if(event.deltaY > 0 && this.state.at < this.state.highest.Height) {
      this.setState(prev => ({at: prev.at + 1, follow: prev.at + 1 === this.state.highest.Height}))
    }
  }

  render() {
    const view = this.viewport()

    const content = <div className="ChainExplorer" onWheel={this.scroll}>{view.map(row => {
      const base = this.state.at - this.state.at % rows
      const className = row === this.state.at ? 'ChainExplorer-at' : (row < base ? 'ChainExplorer-after' : 'ChainExplorer-before')
      let info = <span>(fetching)</span>
      if(this.state.cache[row]) {
        const ts = this.state.cache[row]

        let msgc = -1
        if(ts.Cids[0] && this.state.messages[ts.Cids[0]['/']]) { // TODO: get from all blks
          msgc = this.state.messages[ts.Cids[0]['/']].length
        }
        if(msgc > 0) {
          msgc = <b>{msgc}</b>
        }
        let time = '?'
        if(this.state.cache[row - 1]){
          time = <span>{ts.Blocks[0].Timestamp - this.state.cache[row - 1].Blocks[0].Timestamp}s</span>
        }

        info = <span>
          <BlockLinks cids={ts.Cids} blocks={ts.Blocks} conn={this.props.client} mountWindow={this.props.mountWindow} /> Msgs: {msgc} ΔT:{time}
        </span>
      }

      return <div key={row} className={className}>@{row} {info}</div>
    })}</div>

    return (<Window onClose={this.props.onClose} title={`Chain Explorer ${this.state.follow ? '(Following)' : ''}`}>
      {content}
    </Window>)
  }
}

export default ChainExplorer
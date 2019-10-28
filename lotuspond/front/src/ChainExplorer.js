import React from 'react';
import {BlockLinks} from "./BlockLink";
import Window from "./Window";

const rows = 32

class ChainExplorer extends React.Component {
  fetching = []

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

  async fetch(h, base, cache, msgcache) {
    //console.log(h, base, cache)

    if (this.fetching[h]) {
      return cache[h]
    }
    this.fetching[h] = true

    if (h < 0) {
      return
    }
    if(!base.Blocks) {
      console.log("base for H is nil blk", h, base)
      return
    }
    let cids = base.Blocks.map(b => (b.Parents || []))
        .reduce((acc, val) => {
          let out = {...acc}
          val.forEach(c => out[c['/']] = 8)
          return out
        }, {})
    cids = Object.keys(cids).map(k => ({'/': k}))
    console.log("parents", cids)

    const blocks = await Promise.all(cids.map(cid => this.props.client.call('Filecoin.ChainGetBlock', [cid])))

    if (!blocks[0]) {
      return
    }

    cache[h] = {
      Height: blocks[0].Height,
      Cids: cids,
      Blocks: blocks,
    }

    await this.updateMessages(cids, msgcache)

    return cache[h]
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

    let cache = {...this.state.cache}
    let msgcache = {...this.state.messages}

    console.log("top", top)

    let parent = cache[top]
    for(let i = 0; i < rows; i++) {
      let newts = await this.fetch(top - i, parent, cache, msgcache)
      parent = newts ? newts : parent
    }
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
        let h = <i>{row}</i>
      if(this.state.cache[row]) {
        const ts = this.state.cache[row]

        h = ts.Height

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

      return <div key={row} className={className}>@{h} {info}</div>
    })}</div>

    return (<Window initialSize={{width: 800}} onClose={this.props.onClose} title={`Chain Explorer ${this.state.follow ? '(Following)' : ''}`}>
      {content}
    </Window>)
  }
}

export default ChainExplorer
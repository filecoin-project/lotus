import React from 'react';
import './App.css';

function colForH(besth, height) {
    const diff = besth - height
    if(diff === 0) return '#6f6'
    if(diff === 1) return '#df4'
    if(diff < 4) return '#ff0'
    if(diff < 10) return '#f60'
    return '#f00'
}

function colLag(lag) {
    if(lag < 100) return '#6f6'
    if(lag < 400) return '#df4'
    if(lag < 1000) return '#ff0'
    if(lag < 4000) return '#f60'
    return '#f00'
}

function lagCol(lag, good) {
    return <span>
        <span style={{color: colLag(lag)}}>{lag}</span>
        <span style={{color: good ? '#f0f0f0' : '#f60'}}>ms</span>
    </span>
}

class App extends React.Component {
    constructor(props) {
        super(props);

        let ws = new WebSocket("ws://" + window.location.host + "/sub")
        //let ws = new WebSocket("ws://127.0.0.1:2975/sub")

        ws.onmessage = (ev) => {
            console.log(ev)
            let update = JSON.parse(ev.data)

            update.Update.Weight = Number(update.Update.Weight)

            let wdiff = update.Update.Weight - (this.state[update.From] || {Weight: update.Update.Weight}).Weight
            wdiff = <span style={{color: wdiff < 0 ? '#f00' : '#f0f0f0'}}>{wdiff}</span>

            let utDiff = update.Time - (this.state[update.From] || {utime: update.Time}).utime
            utDiff = <span style={{color: utDiff < 0 ? '#f00' : '#f0f0f0'}}>{utDiff}ms</span>

            this.setState( prev => ({
                ...prev, [update.From]: {...update.Update, utime: update.Time, wdiff: wdiff, utDiff: utDiff},
            }))
        }

        ws.onclose = () => {
            this.setState({disconnected: true})
        }

        this.state = {}
    }

    render() {
        if(this.state.disconnected) {
            return <span>Error: disconnected</span>
        }

        let besth = Object.keys(this.state).map(k => this.state[k]).reduce((p, n) => p > n.Height ? p : n.Height, -1)
        let bestw = Object.keys(this.state).map(k => this.state[k]).reduce((p, n) => p > n.Weight ? p : n.Weight, -1)

        return <table>
            <tr><td>PeerID</td><td>Nickname</td><td>Lag</td><td>Weight(best, prev)</td><td>Height</td><td>Blocks</td></tr>
            {Object.keys(this.state).map(k => [k, this.state[k]]).map(([k, v]) => {
            let mnrs = v.Blocks.map(b => <td>&nbsp;m:{b.Miner}({lagCol(v.Time ? v.Time - (b.Timestamp*1000) : v.utime - (b.Timestamp*1000), v.Time)})</td>)
            let l = [
              <td>{k}</td>,
              <td>{v.NodeName}</td>,
              <td>{v.Time ? lagCol(v.utime - v.Time, true) : ""}(Δ{v.utDiff})</td>,
              <td style={{color: bestw !== v.Weight ? '#f00' : '#afa'}}>{v.Weight}({bestw - v.Weight}, {v.wdiff})</td>,
              <td style={{color: colForH(besth, v.Height)}}>{v.Height}({besth - v.Height})</td>,
              ...mnrs,
            ]

                l = <tr>{l}</tr>
            return l
        })
        }
        </table>
    }
}
export default App;

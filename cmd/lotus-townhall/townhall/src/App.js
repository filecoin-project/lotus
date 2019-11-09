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

class App extends React.Component {
    constructor(props) {
        super(props);

        let ws = new WebSocket("ws://" + window.location.host + "/sub")
        //let ws = new WebSocket("ws://127.0.0.1:2975/sub")

        ws.onmessage = (ev) => {
            console.log(ev)
            let update = JSON.parse(ev.data)

            this.setState( prev => ({
                ...prev, [update.From]: update.Update,
            }))
        }

        this.state = {}
    }

    render() {
        let besth = Object.keys(this.state).map(k => this.state[k]).reduce((p, n) => p > n.Height ? p : n.Height, -1)
        let bestw = Object.keys(this.state).map(k => this.state[k]).reduce((p, n) => p > n.Weight ? p : n.Weight, -1)

        return <table>{Object.keys(this.state).map(k => [k, this.state[k]]).map(([k, v]) => {

            let mnrs = v.Blocks.map(b => <span>&nbsp;m:{b.Miner}</span>)
            let l = [<td>{k}</td>,
                <td>{v.NodeName}</td>,
                <td style={{color: bestw !== v.Weight ? '#f00' : '#afa'}}>{v.Weight}({bestw - v.Weight})</td>,
                <td style={{color: colForH(besth, v.Height)}}>{v.Height}({besth - v.Height})</td>,
                <td>{mnrs}</td>]
                l = <tr>{l}</tr>

            return l
        })
        }</table>
    }
}
export default App;

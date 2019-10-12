import React from 'react';
import './App.css';

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
        let best = Object.keys(this.state).map(k => this.state[k]).reduce((p, n) => p > n.Height ? p : n.Height, -1)
        console.log(best)

        return <table>{Object.keys(this.state).map(k => [k, this.state[k]]).map(([k, v]) => {
            let l = [<td>{k}</td>, <td>{v.NodeName}</td>, <td>{v.Height}</td>]
            if (best !== v.Height) {
                l = <tr style={{color: '#f00'}}>{l}</tr>
            } else {
                l = <tr>{l}</tr>
            }

            return l
        })
        }</table>
    }
}
export default App;

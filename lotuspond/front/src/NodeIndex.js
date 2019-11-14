import React from 'react';
import {Link} from "react-router-dom";
import {Client} from "rpc-websockets";

class Index extends React.Component {
    constructor(props) {
        super(props)

        this.state = {rpcUrl: "ws://127.0.0.1:1234/rpc/v0", rpcToken: '', conns: {}, info: {}}

        const initialState = JSON.parse(window.localStorage.getItem('saved-nodes'))
        if (initialState) {
            this.state.nodes = initialState
        } else {
            this.state.nodes = []
        }
        this.state.nodes.forEach((n, i) => this.connTo(i, n))
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        window.localStorage.setItem('saved-nodes', JSON.stringify(this.state.nodes))
        //this.state.nodes.filter(i => [i, this.state.conns[i]]).forEach(([i, n]) => this.connTo(i, n))
    }

    componentWillUnmount() {
        Object.keys(this.state.conns).forEach(c => this.state.conns[c].close())
    }

    async updateInfo(n) {
        const conn = this.state.conns[n]
        const head = await conn.call('Filecoin.ChainHead',  [])
        const peers = await conn.call('Filecoin.NetPeers',  [])

        this.setState(p => ({info: {...p.info, [n]: {head, peers}}}))
    }

    connTo = async (n, node) => {
        const client = new Client(`${node.addr}?token=${node.token}`)
        client.on('open', async () => {
            this.setState(p => ({conns: {...p.conns, [n]: client}}))
            setInterval(() => this.updateInfo(n), 1333)
        })
    }

    onAdd = () => {
        this.setState({addingNode: true})
    }

    update = (name) => (e) => this.setState({ [name]: e.target.value })

    tokenOk = () => {
        let m = this.state.rpcToken.match(/\.(.+)\./)
        // TODO: eww
        if(m && atob(m[1]) === '{"Allow":["read","write","sign","admin"]}') {
            return (
                <span>-Token OK-
          <div>
            <button onClick={this.addNode}>Add Node</button>
          </div>
        </span>
            )
        }
        return <span>-Expecting valid admin token-</span>
    }

    addNode = async () => {
        this.setState(p => ({nodes: [...p.nodes, {addr: this.state.rpcUrl, token: this.state.rpcToken}], addingNode: true}))
    }

    render() {
        return (
            <div className="Index">
                <div className="Index-nodes">
                    <div>
                        {
                            this.state.nodes.map((node, i) => {
                                let info = <span>[no conn]</span>
                                if (this.state.info[i]) {
                                    const ni = this.state.info[i]
                                    info = <span>H:{ni.head.Height}; Peers:{ni.peers.length}</span>
                                }

                                return <div className="Index-node">
                                    <span>{i}. {node.addr} <Link to={`/app/node/${i}`}>[OPEN UI]</Link> {info}</span>
                                </div>}
                            )
                        }
                    </div>
                    <a hidden={this.state.addingNode} href='#' onClick={this.onAdd} className="Button">[Add Node]</a>
                    <div hidden={!this.state.addingNode}>
                        <div>---------------</div>
                        <div>
                            + RPC:<input defaultValue={"ws://127.0.0.1:1234/rpc/v0"} onChange={this.update("rpcUrl")}/>
                        </div>
                        <div>
                            Token (<code>lotus auth create-token --perm admin</code>): <input onChange={this.update("rpcToken")}/>{this.tokenOk()}
                        </div>
                    </div>
                </div>
                <div className="Index-footer">
                    <div>
                        <Link to={"/app/pond"}>Open Pond</Link>
                    </div>
                </div>
            </div>
        )
    }
}

export default Index

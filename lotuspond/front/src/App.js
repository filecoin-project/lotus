import React from 'react';
import './App.css';
import { BrowserRouter as Router, Route, Link } from "react-router-dom";
import Pond from "./Pond";
import SingleNode from "./SingleNode";

class Index extends React.Component {
  constructor(props) {
    super(props)

    this.state = {rpcUrl: "ws://127.0.0.1:1234/rpc/v0", rpcToken: ''}

    const initialState = JSON.parse(window.localStorage.getItem('saved-nodes'))
    if (initialState) {
      this.state.nodes = initialState
    } else {
      this.state.nodes = []
    }
  }

  componentDidUpdate(prevProps, prevState, snapshot) {
    window.localStorage.setItem('saved-nodes', JSON.stringify(this.state.nodes))
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
              this.state.nodes.map((node, i) => <div className="Index-node">
                <span>{i}. {node.addr} <Link to={`/app/node/${i}`}>[OPEN UI]</Link></span>
              </div>)
            }
          </div>
          <a hidden={this.state.addingNode} href='#' onClick={this.onAdd} className="Button">[Add Node]</a>
          <div hidden={!this.state.addingNode}>
            <div>---------------</div>
            <div>
              + RPC:<input defaultValue={"ws://127.0.0.1:1234/rpc/v0"} onChange={this.update("rpcUrl")}/>
            </div>
            <div>
              Token (<code>lotus auth create-admin-token</code>): <input onChange={this.update("rpcToken")}/>{this.tokenOk()}
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

class App extends React.Component {
  constructor(props) {
    super(props)
  }

  render() {
    return (
      <Router>
        <Route path="/" exact component={Index} />
        <Route path="/app/pond/" component={Pond} />
        <Route path="/app/node/:node" component={SingleNode} />
      </Router>
    )
  }
}

export default App

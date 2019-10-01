import React from 'react';
import './App.css';
import { BrowserRouter as Router, Route, Link } from "react-router-dom";
import Pond from "./Pond";
import SingleNode from "./SingleNode";
import Index from "./NodeIndex";

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

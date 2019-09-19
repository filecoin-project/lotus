import React from 'react'
import {Cristal} from "react-cristal";

class Window extends React.Component {
  render() {
    let props = {className: '', ...this.props}
    props.className = `${props.className} Window`

    return <Cristal {...props}>
      {this.props.children}
    </Cristal>
  }
}

export default Window

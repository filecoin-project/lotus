import React from "react";

function filStr(raw) {
  if(typeof raw !== 'string') {
    raw = String(raw)
  }
  if(raw.length < 19) {
    raw = '0'.repeat(19 - raw.length).concat(raw)
  }

  let out = raw.substring(0, raw.length - 18).concat('.', raw.substring(raw.length - 18, raw.length)).replace(/\.0+$|0+$/g, '');
  return out ? out : '0'
}


class Fil extends React.Component {
  render() {
    return filStr(this.props.children)
  }
}

export default Fil
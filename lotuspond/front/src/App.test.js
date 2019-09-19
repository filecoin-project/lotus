import React from 'react';
import ReactDOM from 'react-dom';
import Pond from './Pond';

it('renders without crashing', () => {
  const div = document.createElement('div');
  ReactDOM.render(<Pond />, div);
  ReactDOM.unmountComponentAtNode(div);
});

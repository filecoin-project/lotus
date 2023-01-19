// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract DelegatecallActor {
    uint public counter;

    function getCounter() public view returns (uint){
	return counter;
    }
    function setVars(uint _counter) public payable {
        counter = _counter;
    }
}

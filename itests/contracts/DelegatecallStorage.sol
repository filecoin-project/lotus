// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract DelegatecallStorage {
    uint public counter;

    function setVars(address _contract, uint _counter) public payable returns (uint){
         (bool success, ) = _contract.delegatecall(
            abi.encodeWithSignature("setVars(uint256)", _counter)
        );
        require(success, 'failed');
	return counter;
    }
}

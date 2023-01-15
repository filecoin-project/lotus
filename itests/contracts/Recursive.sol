// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract Recursive {
    event RecursiveCallEvent(uint256 count);

    function recursive10() public returns (uint256){
	recursiveCall(10);
    }
    function recursive2() public returns (uint256){
	recursiveCall(2);
    }
    function recursive1() public returns (uint256){
	recursiveCall(1);
    }
    function recursive0() public returns (uint256){
	recursiveCall(0);
    }
    function recursiveCall(uint256 count) public returns (uint256) {
        emit RecursiveCallEvent(count);
        if (count > 1) {
            recursiveCall(count--);
        }
	return count;
    }
}

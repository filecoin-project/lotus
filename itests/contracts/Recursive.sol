// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract Recursive {
    event RecursiveCallEvent(uint256 count);

    function recursive10() public returns (uint256){
      return recursiveCall(10);
    }
    function recursive2() public returns (uint256){
      return recursiveCall(2);
    }
    function recursive1() public returns (uint256){
      return recursiveCall(1);
    }
    function recursive0() public returns (uint256){
      return recursiveCall(0);
    }
    function recursiveCall(uint256 count) public returns (uint256) {
        if (count > 0) {
            emit RecursiveCallEvent(count);
            return recursiveCall(count-1);
        }
        return count;
    }
}

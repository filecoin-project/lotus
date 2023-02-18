// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract StackSelf {
    function exec1(uint256 n) public payable {
        if(n == 0) {
            return;
        }

        exec1(n-1);
    }
}
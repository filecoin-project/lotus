
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract StackRecCallExp {
    function exec1(uint256 r) public payable {
        if(r > 0) {
            StackRecCallExp(address(this)).exec1(r-1);
        }

        return;
    }
}

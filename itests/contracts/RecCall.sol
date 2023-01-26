// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract StackRecCall {
    function exec1(uint256 n, uint256 m, uint256 r) public payable {
        if(n == 0) {
            if(r > 0) {
                StackRecCall(address(this)).exec1(m, m, r-1);
            }

            return;
        }

        exec1(n-1, m, r);
    }
}

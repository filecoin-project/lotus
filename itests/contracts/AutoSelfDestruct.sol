// SPDX-License-Identifier: MIT
pragma solidity >=0.8.17;

contract AutoSelfDestruct {
    constructor() {
        destroy();
    }
    function destroy() public {
        selfdestruct(payable(msg.sender));
    }
}

// SPDX-License-Identifier: MIT
pragma solidity ^0.8.2;


contract Test_contract {
    uint256 public number;

    constructor(uint256 _number) {
        number = _number;
    }

    function get_number() public view returns (uint256) {
        return number;
    }
}

contract App {

    event NewTest(address sender, uint256 number);

    function new_Test(uint256 number)
        public
        returns (address)
    {
        address mynew = address(new Test_contract({_number: number}));
        emit NewTest(tx.origin, number);
        return mynew;
    }
}

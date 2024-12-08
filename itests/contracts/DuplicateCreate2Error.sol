// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract Child {
    uint256 public value;
    
    constructor(uint256 _value) {
        value = _value;
    }
}

contract DuplicateCreate2Error {
    function deployTwice() public {
        bytes32 salt = bytes32(uint256(0));
        
        // First deployment
        new Child{salt: salt}(42);
        
        // Second deployment with same salt - should fail
        new Child{salt: salt}(42);
    }
}
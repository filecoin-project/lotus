// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

// Interface for ContractB
interface IContractB {
    function callBackAndRead(address origin) external view returns (uint256);
    function callBackAndDouble(address origin) external view returns (uint256);
}

// ContractA: Has state and can call ContractB which calls back
contract ContractA {
    uint256 public storedValue;
    address public contractB;

    constructor() {
        storedValue = 42;
    }

    // Sets the ContractB address (called after deployment)
    function setContractB(address _contractB) external {
        contractB = _contractB;
    }

    // Simple view function that ContractB will call back
    function getValue() external view returns (uint256) {
        return storedValue;
    }

    // Sets a new value
    function setValue(uint256 _value) external {
        storedValue = _value;
    }

    // Calls ContractB, which calls back to this contract's getValue()
    // This tests: A calls B, B calls A.getValue() (view)
    function callBAndReadBack() external view returns (uint256) {
        return IContractB(contractB).callBackAndRead(address(this));
    }

    // Calls ContractB, which reads and doubles our value
    function callBAndDouble() external view returns (uint256) {
        return IContractB(contractB).callBackAndDouble(address(this));
    }

    // Returns the ContractB address for verification
    function getContractB() external view returns (address) {
        return contractB;
    }
}

// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

// Interface for ContractA (used by ContractB to call back)
interface IContractA {
    function getValue() external view returns (uint256);
}

// ContractB: Receives a call and calls back to the origin contract's view function
contract ContractB {
    // Calls back to the provided contract address and reads its getValue()
    function callBackAndRead(address origin) external view returns (uint256) {
        return IContractA(origin).getValue();
    }

    // Calls back to origin, reads value, and returns double
    function callBackAndDouble(address origin) external view returns (uint256) {
        uint256 val = IContractA(origin).getValue();
        return val * 2;
    }
}

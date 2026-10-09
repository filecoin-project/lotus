// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract GasDependent {
    // Reverts unless more than threshold gas remains, so enough gas turns a revert into success.
    function requireGas(uint256 threshold) external view {
        require(gasleft() > threshold, "not enough gas");
    }
}

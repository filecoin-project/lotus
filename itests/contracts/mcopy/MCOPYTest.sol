// SPDX-License-Identifier: MIT
pragma solidity ^0.8.26;

contract MCOPYTest {
    function optimizedCopy(bytes memory data) public pure returns (bytes memory) {
        bytes memory result = new bytes(data.length);
        assembly {
            let length := mload(data) // Get the length of the input data
            let source := add(data, 0x20) // Point to the start of the data (skip the length)
            let destination := add(result, 0x20) // Point to the start of the result memory (skip the length)

            // Use MCOPY opcode directly for memory copying
            // destination: destination memory pointer
            // source: source memory pointer
            // length: number of bytes to copy
            mcopy(destination, source, length)
        }
        return result;
    }

}

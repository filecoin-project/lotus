// SPDX-License-Identifier: MIT
pragma solidity >=0.8.17;

contract BlockTest {

    function testChainID() public view{
        require(block.chainid == 314);
    }

    function getChainID() public view returns (uint256){
        return block.chainid;
    }

    function getBlockhash() public view returns (bytes32) {
        return blockhash(block.number);
    }
    function getBasefee() public view returns (uint256){
        return block.basefee;
    }
    function getBlockNumber() public view returns (uint256){
        return block.number;
    }
    function getTimestamp() public view returns (uint256){
        return block.timestamp;
    }
}

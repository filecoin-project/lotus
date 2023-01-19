// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract GasLimitTest {
    address payable receiver;
    constructor(){
        address mynew = address(new GasLimitTestReceiver());
        receiver = payable(mynew);
    }
    function send() public payable{
        receiver.transfer(msg.value);
    }
    function expensiveTest() public{
        GasLimitTestReceiver(receiver).expensive();
    }
}

contract GasLimitTestReceiver {
    uint256[] data;
    fallback() external payable {
        expensive();
    }
    function expensive() public{
        for (uint256 i = 0; i < 100; i++) {
            data.push(i);
        }
    }
    function getDataLength() public view returns (uint256) {
        return data.length;
    }
}

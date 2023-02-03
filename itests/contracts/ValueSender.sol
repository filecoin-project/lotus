// SPDX-License-Identifier: MIT
pragma solidity >=0.8.17;

contract A {
    event LogCreateB(address _bAddress);
    event LogSendEthA(address _bAddress, uint _value);
    event LogReceiveEthA(address _bAddress, uint _value);

    // Function to create a new instance of contract B and return its address
    function createB() public returns (address) {
        address bAddress = address(new B());
        emit LogCreateB(bAddress);
        return bAddress;
    }

    // Payable method to accept eth and an address for B and send the eth to B
    function sendEthToB(address payable _bAddress) public payable {
        emit LogSendEthA(_bAddress, msg.value);
        _bAddress.transfer(msg.value);
    }

    // Payable function to accept the eth
    receive() external payable {
        emit LogSendEthA(msg.sender, msg.value);
    }
}

contract B {
    event LogSelfDestruct();
    event LogSendEthToA(address _to, uint _value);
    event LogReceiveEth(address from, uint value);
    address payable creator;

    constructor(){
        creator = payable(msg.sender);
    }

    // Payable function to accept the eth
    receive() external payable {
      emit LogReceiveEth(msg.sender, msg.value);
    }

    // Method to send ether to contract A
    function sendEthToA() public payable {
        emit LogSendEthToA(creator,address(this).balance);
        creator.transfer(address(this).balance);
    }

    // Self destruct method to send eth to its creator
    function selfDestruct() public {
        emit LogSelfDestruct();
        selfdestruct(creator);
    }
}

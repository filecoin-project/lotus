// SPDX-License-Identifier: MIT
pragma solidity >=0.8.17;

 

contract DeployValueTest {
    address public newContract;

    constructor() payable { 
        newContract = address(new NewContract{value: msg.value}());
    }

    function getConst() public view returns (uint) {
        return 7;
    }
    
    function getNewContractBalance() public view returns (uint) {
        return NewContract(newContract).getBalance();
    }
}

contract NewContract {
    constructor() payable {
    }

    function getBalance() public view returns (uint) {
        return address(this).balance;
    }
}

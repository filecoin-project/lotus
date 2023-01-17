pragma solidity ^0.8.0;

contract  DelegateCallGasConsumption {
    address targetContract;
    uint gasUsed;

    function setTarget(address _target) public {
        targetContract = _target;
    }

    function callTarget() public {
        // Measure the gas consumption before the delegatecall
        gasUsed = gasleft();
        // Delegate call to target contract
        targetContract.delegatecall(msg.data);
        // Measure the gas consumption after the delegatecall
        gasUsed = gasUsed - gasleft();
    }

    function getGasUsed() public view returns (uint) {
        return gasUsed;
    }
}

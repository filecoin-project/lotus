// SPDX-License-Identifier: MIT
pragma solidity >=0.8.17;
contract Create2Factory {

    bytes32 savedSalt;

    // Returns the address of the newly deployed contract
    function deploy(
        bytes32 _salt
    ) public returns (address) {
        // This syntax is a newer way to invoke create2 without assembly, you just need to pass salt
        // https://docs.soliditylang.org/en/latest/control-structures.html#salted-contract-creations-create2
        savedSalt = _salt;
        (bool success, address ret) = deployDelegateCall(_salt);
        require(success);
        return ret;
    }

    function deployDelegateCall(
        bytes32 _salt
    ) public returns (bool, address) {
        bytes memory data = abi.encodeWithSignature("_deploy(bytes32)", _salt);
        (bool success, bytes memory returnedData) = address(this).delegatecall(data);
        if(success){
            (address ret) = abi.decode(returnedData, (address));
            return (success, ret);
        }else{
            return (success, address(0));
        }
    }

    function _deploy(bytes32 _salt) public returns (address) {
        // https://solidity-by-example.org/app/create1/
        // This syntax is a newer way to invoke create2 without assembly, you just need to pass salt
        // https://docs.soliditylang.org/en/latest/control-structures.html#salted-contract-creations-create2
        return address(new SelfDestruct{salt: _salt}(_salt));
    }

    function test(address _address) public returns (address){

      // run destroy() on _address
      SelfDestruct selfDestruct = SelfDestruct(_address);
      selfDestruct.destroy();

      //verify data can still be accessed
      address ret = selfDestruct.sender();

      // attempt and fail to deploy contract using salt
      (bool success, ) = deployDelegateCall(selfDestruct.salt());
      require(!success);

      return ret;
    }
}

contract SelfDestruct {
    address public sender;
    bytes32 public salt;
    constructor(bytes32 _salt) {
       sender = tx.origin;
       salt=_salt;
    }
    function destroy() public {
        selfdestruct(payable(msg.sender));
    }
}


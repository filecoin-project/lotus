// SPDX-License-Identifier: MIT
pragma solidity >=0.8.17;

contract MultipleEventEmitter {
    // Define events
    event Event1(address indexed sender, uint256 timestamp);
    event Event2(string message);
    event Event3(uint256 value);
    event Event4(bool flag);
    event Event5(address indexed recipient);
    event Event6(uint256 indexed id, string data);
    event Event7(bytes32 hash);

    // Function that emits four events
    function emitFourEvents() public {
        emit Event1(msg.sender, block.timestamp);
        emit Event2("Four events function called");
        emit Event3(42);
        emit Event4(true);
    }

    // Function that emits three events
    function emitThreeEvents(address _recipient, uint256 _id) public {
        emit Event5(_recipient);
        emit Event6(_id, "Three events function called");
        emit Event7(keccak256(abi.encodePacked(_recipient, _id)));
    }
}
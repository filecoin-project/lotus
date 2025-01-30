// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

contract TransientStorageTest {
    constructor() {
    }

    // Test 0: Initial Constructor Test
    function runTests() public returns (bool) {
        _runTests();
    }

    function _runTests() internal {
        testBasicFunctionality();
        testLifecycleValidation();
    }

    // Test 1: Basic Functionality
    function testBasicFunctionality() public {
        uint256 slot = 1;
        uint256 value = 42;

        // Store value using TSTORE
        assembly {
            tstore(slot, value)
        }

        // Retrieve value using TLOAD
        uint256 retrievedValue;
        assembly {
            retrievedValue := tload(slot)
        }

        require(retrievedValue == value, "TLOAD did not retrieve the correct value");

        // Verify TLOAD from uninitialized location
        uint256 uninitializedSlot = 2;
        uint256 uninitializedValue;
        assembly {
            uninitializedValue := tload(uninitializedSlot)
        }

        require(uninitializedValue == 0, "Uninitialized TLOAD did not return zero");
    }

    // Test 2.1: Verify transient storage clears after transaction
    function testLifecycleValidation() public {
        uint256 slot = 3;
        uint256 value = 99;

        // Store value using TSTORE
        assembly {
            tstore(slot, value)
        }

        // Verify it exists within the same transaction
        uint256 retrievedValue;
        assembly {
            retrievedValue := tload(slot)
        }
        require(retrievedValue == value, "TLOAD did not retrieve stored value within transaction");
    }

    // Test 2.2: Verify transient storage clears in subsequent transactions
    function testLifecycleValidationSubsequentTransaction() public {
        uint256 slot = 3;
        bool cleared = isStorageCleared(slot);
        require(cleared, "Transient storage was not cleared after transaction");
    }

    // Utility Function: Check if transient storage is cleared
    function isStorageCleared(uint256 slot) public view returns (bool) {
        uint256 retrievedValue;
        assembly {
            retrievedValue := tload(slot)
        }
        return retrievedValue == 0; // True if cleared
    }

    // Test 3: Verify nested contract independence
    function testNestedContracts(address other) public returns (bool) {
        uint256 slot = 4;
        uint256 value = 88;

        TransientStorageTest nested = TransientStorageTest(other);

        // Store in this contract's transient storage
        assembly {
            tstore(slot, value)
        }

        // Call nested contract to write its own transient storage
        nested.writeTransientData(slot, 123);

        // Verify this contract's data is unchanged
        uint256 retrievedValue;
        assembly {
            retrievedValue := tload(slot)
        }
        require(retrievedValue == value, "Nested contract interfered with this contract's storage");

        // Verify nested contract's data independently
        uint256 nestedValue = nested.readTransientData(slot);
        require(nestedValue == 123, "Nested contract data incorrect");
        return true;
    }

    // Test 4: Reentry scenario
    function testReentry(address otherContract) public returns (bool) {
        uint256 slot = 5;
        uint256 value = 123;

        // Store a value in transient storage
        assembly {
            tstore(slot, value)
        }

        // Call the other contract to trigger a callback to this contract
        TransientStorageTest(otherContract).reentryCallback();

        // After reentry, check that the transient storage still has the correct value
        uint256 retrievedValue;
        assembly {
            retrievedValue := tload(slot)
        }

        require(retrievedValue == value, "Reentry altered transient storage");
        return true;
    }

    // Utility Function for Test 4: Reentry callback
    function reentryCallback() public {
        uint256 slot = 6;
        uint256 value = 456;

        // Store a different value in a different slot
        assembly {
            tstore(slot, value)
        }

        // Verify the value was stored correctly
        uint256 retrievedValue;
        assembly {
            retrievedValue := tload(slot)
        }

        require(retrievedValue == value, "Reentry callback failed to store correct value");
    }

    // Utility Function for Test 3: Write to transient storage
    function writeTransientData(uint256 slot, uint256 value) external {
        assembly {
            tstore(slot, value)
        }
    }

    // Utility Function for Test 3: Read from transient storage
    function readTransientData(uint256 slot) external view returns (uint256) {
        uint256 value;
        assembly {
            value := tload(slot)
        }
        return value;
    }
}

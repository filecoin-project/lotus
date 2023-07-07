// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract Errors {
    error CustomError();

    function failRevertEmpty() public  {
        revert();
    }
    function failRevertReason() public  {
        revert("my reason");
    }
    function failAssert() public  {
        assert(false);
    }
    function failDivZero() public  {
        int a = 1;
        int b = 0;
        a / b;
    }
    function failCustom() public  {
        revert CustomError();
    }
}

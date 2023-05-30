// SPDX-License-Identifier: MIT
pragma solidity >=0.8.17;

//sending eth should fall because fallback is not payable
contract NotPayable {
    fallback() external {}
}

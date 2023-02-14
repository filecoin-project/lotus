// SPDX-License-Identifier: MIT
pragma solidity >=0.8.17;

contract GetDifficulty {
  function getDifficulty () public view returns (uint256) {
    return block.difficulty;
  }
}


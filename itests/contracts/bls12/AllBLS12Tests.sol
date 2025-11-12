// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./G1AddTest.sol";
import "./G1MsmTest.sol";
import "./G2AddTest.sol";
import "./G2MsmTest.sol";
import "./MapFpToG1Test.sol";
import "./MapFp2ToG2Test.sol";
import "./PairingTest.sol";

contract AllBLS12Tests {
    G1AddTest private g1Add;
    G1MsmTest private g1Msm;
    G2AddTest private g2Add;
    G2MsmTest private g2Msm;
    MapFpToG1Test private mapFpToG1;
    MapFp2ToG2Test private mapFp2ToG2;
    PairingTest private pairing;

    constructor() {
        g1Add = new G1AddTest();
        g1Msm = new G1MsmTest();
        g2Add = new G2AddTest();
        g2Msm = new G2MsmTest();
        mapFpToG1 = new MapFpToG1Test();
        mapFp2ToG2 = new MapFp2ToG2Test();
        pairing = new PairingTest();
    }

    function runTests() public view {
        g1Add.runTests();
        g1Msm.runTests();
        g2Add.runTests();
        g2Msm.runTests();
        mapFpToG1.runTests();
        mapFp2ToG2.runTests();
        pairing.runTests();
    }
}


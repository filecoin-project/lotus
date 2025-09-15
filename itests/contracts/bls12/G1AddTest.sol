// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract G1AddTest {
    function callOk(address pc, bytes memory input, uint256 expectedLen) internal view returns (bytes memory) {
        (bool ok, bytes memory out) = pc.staticcall(input);
        require(ok, "precompile reverted");
        require(out.length == expectedLen, "unexpected return length");
        return out;
    }

    function expectRevert(address pc, bytes memory input) internal view {
        (bool ok, ) = pc.staticcall(input);
        require(!ok, "expected revert");
    }

    function expectEq(bytes memory a, bytes memory b, string memory what) internal pure {
        require(keccak256(a) == keccak256(b), what);
    }

    address constant PRECOMPILE = 0x000000000000000000000000000000000000000b; // BLS12_G1ADD
    uint256 constant OUT_LEN = 128;

    // Vector: g1 + p1
    bytes constant INPUT_ADD_G1_P1 = hex"0000000000000000000000000000000017f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb0000000000000000000000000000000008b3f481e3aaa0f1a09e30ed741d8ae4fcf5e095d5d00af600db18cb2c04b3edd03cc744a2888ae40caa232946c5e7e100000000000000000000000000000000112b98340eee2777cc3c14163dea3ec97977ac3dc5c70da32e6e87578f44912e902ccef9efe28d4a78b8999dfbca942600000000000000000000000000000000186b28d92356c4dfec4b5201ad099dbdede3781f8998ddf929b4cd7756192185ca7b8f4ef7088f813270ac3d48868a21";
    bytes constant EXPECT_ADD_G1_P1 = hex"000000000000000000000000000000000a40300ce2dec9888b60690e9a41d3004fda4886854573974fab73b046d3147ba5b7a5bde85279ffede1b45b3918d82d0000000000000000000000000000000006d3d887e9f53b9ec4eb6cedf5607226754b07c01ace7834f57f3e7315faefb739e59018e22c492006190fba4a870025";
    // Vector: g1 + g1 = 2*g1
    bytes constant INPUT_G1_PLUS_G1 = hex"0000000000000000000000000000000017f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb0000000000000000000000000000000008b3f481e3aaa0f1a09e30ed741d8ae4fcf5e095d5d00af600db18cb2c04b3edd03cc744a2888ae40caa232946c5e7e10000000000000000000000000000000017f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb0000000000000000000000000000000008b3f481e3aaa0f1a09e30ed741d8ae4fcf5e095d5d00af600db18cb2c04b3edd03cc744a2888ae40caa232946c5e7e1";
    bytes constant EXPECT_G1_PLUS_G1 = hex"000000000000000000000000000000000572cbea904d67468808c8eb50a9450c9721db309128012543902d0ac358a62ae28f75bb8f1c7c42c39a8c5529bf0f4e00000000000000000000000000000000166a9d8cabc673a322fda673779d8e3822ba3ecb8670e461f73bb9021d5fd76a4c56d9d4cd16bd1bba86881979749d28";
    // Vector: p1 + p1 = 2*p1
    bytes constant INPUT_P1_PLUS_P1 = hex"00000000000000000000000000000000112b98340eee2777cc3c14163dea3ec97977ac3dc5c70da32e6e87578f44912e902ccef9efe28d4a78b8999dfbca942600000000000000000000000000000000186b28d92356c4dfec4b5201ad099dbdede3781f8998ddf929b4cd7756192185ca7b8f4ef7088f813270ac3d48868a2100000000000000000000000000000000112b98340eee2777cc3c14163dea3ec97977ac3dc5c70da32e6e87578f44912e902ccef9efe28d4a78b8999dfbca942600000000000000000000000000000000186b28d92356c4dfec4b5201ad099dbdede3781f8998ddf929b4cd7756192185ca7b8f4ef7088f813270ac3d48868a21";
    bytes constant EXPECT_P1_PLUS_P1 = hex"0000000000000000000000000000000015222cddbabdd764c4bee0b3720322a65ff4712c86fc4b1588d0c209210a0884fa9468e855d261c483091b2bf7de6a630000000000000000000000000000000009f9edb99bc3b75d7489735c98b16ab78b9386c5f7a1f76c7e96ac6eb5bbde30dbca31a74ec6e0f0b12229eecea33c39";
    // Encoded canonical points (128 bytes each) for identity tests
    bytes constant G1_ENC = hex"0000000000000000000000000000000017f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb0000000000000000000000000000000008b3f481e3aaa0f1a09e30ed741d8ae4fcf5e095d5d00af600db18cb2c04b3edd03cc744a2888ae40caa232946c5e7e1";
    bytes constant P1_ENC = hex"00000000000000000000000000000000112b98340eee2777cc3c14163dea3ec97977ac3dc5c70da32e6e87578f44912e902ccef9efe28d4a78b8999dfbca942600000000000000000000000000000000186b28d92356c4dfec4b5201ad099dbdede3781f8998ddf929b4cd7756192185ca7b8f4ef7088f813270ac3d48868a21";
    // Subtraction (add inverse) vectors -> infinity
    bytes constant INPUT_G1_MINUS_G1 = hex"0000000000000000000000000000000017f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb0000000000000000000000000000000008b3f481e3aaa0f1a09e30ed741d8ae4fcf5e095d5d00af600db18cb2c04b3edd03cc744a2888ae40caa232946c5e7e10000000000000000000000000000000017f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb00000000000000000000000000000000114d1d6855d545a8aa7d76c8cf2e21f267816aef1db507c96655b9d5caac42364e6f38ba0ecb751bad54dcd6b939c2ca";
    bytes constant INPUT_P1_MINUS_P1 = hex"00000000000000000000000000000000112b98340eee2777cc3c14163dea3ec97977ac3dc5c70da32e6e87578f44912e902ccef9efe28d4a78b8999dfbca942600000000000000000000000000000000186b28d92356c4dfec4b5201ad099dbdede3781f8998ddf929b4cd7756192185ca7b8f4ef7088f813270ac3d48868a2100000000000000000000000000000000112b98340eee2777cc3c14163dea3ec97977ac3dc5c70da32e6e87578f44912e902ccef9efe28d4a78b8999dfbca9426000000000000000000000000000000000195e911162921ba5ed055b496420f197693d36569ec34c63d7c0529a097d49e543070afba4b707e878e53c2b779208a";

    function runTests() public view {
        // Positive: addition
        bytes memory out1 = callOk(PRECOMPILE, INPUT_ADD_G1_P1, OUT_LEN);
        expectEq(out1, EXPECT_ADD_G1_P1, "g1add add mismatch");

        // Additional positives
        bytes memory out2 = callOk(PRECOMPILE, INPUT_G1_PLUS_G1, OUT_LEN);
        expectEq(out2, EXPECT_G1_PLUS_G1, "g1add doubling g1 mismatch");

        bytes memory out3 = callOk(PRECOMPILE, INPUT_P1_PLUS_P1, OUT_LEN);
        expectEq(out3, EXPECT_P1_PLUS_P1, "g1add doubling p1 mismatch");

        // Identity: g1 + 0 = g1; p1 + 0 = p1
        bytes memory out4 = callOk(PRECOMPILE, bytes.concat(G1_ENC, new bytes(OUT_LEN)), OUT_LEN);
        expectEq(out4, G1_ENC, "g1add identity g1+0 mismatch");

        bytes memory out5 = callOk(PRECOMPILE, bytes.concat(P1_ENC, new bytes(OUT_LEN)), OUT_LEN);
        expectEq(out5, P1_ENC, "g1add identity p1+0 mismatch");

        // Subtraction to infinity
        bytes memory out6 = callOk(PRECOMPILE, INPUT_G1_MINUS_G1, OUT_LEN);
        expectEq(out6, new bytes(OUT_LEN), "g1add g1-g1 != 0");

        bytes memory out7 = callOk(PRECOMPILE, INPUT_P1_MINUS_P1, OUT_LEN);
        expectEq(out7, new bytes(OUT_LEN), "g1add p1-p1 != 0");

        // Negative: incorrect input size should revert
        expectRevert(PRECOMPILE, hex"");
    }
}

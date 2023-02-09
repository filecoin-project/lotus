// SPDX-License-Identifier: MIT
pragma solidity ^0.8.2;
pragma experimental ABIEncoderV2;

contract Test_contract {
    uint256 timestamp;
    address sender;
    string text;
    uint256 number;

    constructor(string memory _text, uint256 _number) {
        sender = tx.origin;
        timestamp = block.timestamp;
        text = _text;
        number = _number;
    }

    function getall()
        public
        view
        returns (
            address,
            uint256,
            address,
            string memory,
            uint256
        )
    {
        return (address(this), timestamp, sender, text, number);
    }

    function get_timestamp() public view returns (uint256) {
        return timestamp;
    }

    function get_sender() public view returns (address) {
        return sender;
    }

    function get_text() public view returns (string memory) {
        return text;
    }

    function get_number() public view returns (uint256) {
        return number;
    }
}

contract App {
    address[] Test_list;
    uint256 Test_list_length;

    function get_Test_list_length() public view returns (uint256) {
        return Test_list_length;
    }

    struct Test_getter {
        address _address;
        uint256 timestamp;
        address sender;
        string text;
        uint256 number;
    }

    function get_Test_N(uint256 index)
        public
        view
        returns (
            address,
            uint256,
            address,
            string memory,
            uint256
        )
    {
        return Test_contract(Test_list[index]).getall();
    }

    function get_first_Test_N(uint256 count, uint256 offset)
        public
        view
        returns (Test_getter[] memory)
    {
        Test_getter[] memory getters = new Test_getter[](count);
        for (uint256 i = offset; i < count; i++) {
            Test_contract myTest = Test_contract(Test_list[i + offset]);
            getters[i - offset]._address = address(myTest);
            getters[i - offset].timestamp = myTest.get_timestamp();
            getters[i - offset].sender = myTest.get_sender();
            getters[i - offset].text = myTest.get_text();
            getters[i - offset].number = myTest.get_number();
        }
        return getters;
    }

    function get_last_Test_N(uint256 count, uint256 offset)
        public
        view
        returns (Test_getter[] memory)
    {
        Test_getter[] memory getters = new Test_getter[](count);
        for (uint256 i = 0; i < count; i++) {
            Test_contract myTest =
                Test_contract(Test_list[Test_list_length - i - offset - 1]);
            getters[i]._address = address(myTest);

            getters[i].timestamp = myTest.get_timestamp();
            getters[i].sender = myTest.get_sender();
            getters[i].text = myTest.get_text();
            getters[i].number = myTest.get_number();
        }
        return getters;
    }

    function get_Test_user_length(address user) public view returns (uint256) {
        return user_map[user].Test_list_length;
    }

    function get_Test_user_N(address user, uint256 index)
        public
        view
        returns (
            address,
            uint256,
            address,
            string memory,
            uint256
        )
    {
        return Test_contract(user_map[user].Test_list[index]).getall();
    }

    function get_last_Test_user_N(
        address user,
        uint256 count,
        uint256 offset
    ) public view returns (Test_getter[] memory) {
        Test_getter[] memory getters = new Test_getter[](count);

        for (uint256 i = offset; i < count; i++) {
            getters[i - offset]._address = user_map[user].Test_list[i + offset];
            getters[i - offset].timestamp = Test_contract(
                user_map[user].Test_list[i + offset]
            )
                .get_timestamp();
            getters[i - offset].sender = Test_contract(
                user_map[user].Test_list[i + offset]
            )
                .get_sender();
            getters[i - offset].text = Test_contract(
                user_map[user].Test_list[i + offset]
            )
                .get_text();
            getters[i - offset].number = Test_contract(
                user_map[user].Test_list[i + offset]
            )
                .get_number();
        }
        return getters;
    }

    struct UserInfo {
        address owner;
        bool exists;
        address[] Test_list;
        uint256 Test_list_length;
    }
    mapping(address => UserInfo) public user_map;
    address[] UserInfoList;
    uint256 UserInfoListLength;

    event NewTest(address sender);

    function new_Test(string memory text, uint256 number)
        public
        returns (address)
    {
        address mynew =
            address(new Test_contract({_text: text, _number: number}));

        if (!user_map[tx.origin].exists) {
            user_map[tx.origin] = create_user_on_new_Test(mynew);
        }
        user_map[tx.origin].Test_list.push(mynew);

        user_map[tx.origin].Test_list_length += 1;

        Test_list.push(mynew);
        Test_list_length += 1;

        emit NewTest(tx.origin);

        return mynew;
    }

    function create_user_on_new_Test(address addr)
        private
        returns (UserInfo memory)
    {
        address[] memory Test_list_;

        UserInfoList.push(addr);
        return
            UserInfo({
                exists: true,
                owner: addr,
                Test_list: Test_list_,
                Test_list_length: 0
            });
    }
}

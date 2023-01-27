// SPDX-License-Identifier: MIT
pragma solidity ^0.8.17;

contract PayableEventsOnTransfer {
    address payable public emitOnce;
    address payable public emitTwice;
    address payable public emitTwiceBlankEvent;
    address payable public emitTwiceOneVar;
    address payable public emitOnceManyVars;
    address payable public emitOnceTooManyVars;

    constructor() payable{ 
        emitOnce = payable(new EmitOnce());
        emitTwice = payable(new EmitTwice());
        emitTwiceBlankEvent = payable(new EmitTwiceBlankEvent());
        emitTwiceOneVar = payable(new EmitTwiceOneVar());

        emitOnceManyVars = payable( new EmitOnceManyVars());
        emitOnceTooManyVars = payable( new EmitOnceTooManyVars());

    }
    function doTransferEmitOnce() public payable {
         emitOnce.transfer(msg.value);
    }

    function doTransferEmitTwiceOneVar() public payable {
         emitTwiceOneVar.transfer(msg.value);
    }
    function doTransferEmitTwice() public payable {
         emitTwice.transfer(msg.value);
    }
    function doTransferEmitTwiceBlankEvent() public payable {
         emitTwiceBlankEvent.transfer(msg.value);
    }
    function doTransferEmitOnceManyVars() public payable {
         emitOnceManyVars.transfer(msg.value);
    }
    function doTransferEmitOnceTooManyVars() public payable {
         emitOnceTooManyVars.transfer(msg.value);
    }

}

// succeeds
contract EmitOnce {
    event ReceivedFunds(address sender, uint256 value);

    fallback() external payable {
       emit ReceivedFunds(msg.sender, msg.value);
    }
}

// fails
contract EmitTwice {
    event ReceivedFunds(address sender, uint256 value);
    fallback() external payable {
            emit ReceivedFunds(msg.sender, msg.value);
            emit ReceivedFunds(msg.sender, msg.value);
    }
}

// succeeds !!
contract EmitTwiceBlankEvent {
    event ReceivedFunds();
    fallback() external payable {
            emit ReceivedFunds();
            emit ReceivedFunds();
    }
}

// fails
contract EmitTwiceOneVar {
    event ReceivedFunds(address sender);
    fallback() external payable {
            emit ReceivedFunds(msg.sender);
            emit ReceivedFunds(msg.sender);
    }
}


// succeeds
contract EmitOnceManyVars {
    event ReceivedFunds(address sender, address sender2, address sender3);
    fallback() external payable {
            emit ReceivedFunds(msg.sender, msg.sender, msg.sender);
    }
}

// fails
contract EmitOnceTooManyVars {
    event ReceivedFunds(address sender, address sender2, address sender3, address sender4);
    fallback() external payable {
            emit ReceivedFunds(msg.sender, msg.sender, msg.sender, msg.sender);
    }
}

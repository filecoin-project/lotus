// SPDX-License-Identifier: MIT
pragma solidity >=0.5.0;

contract EventMatrix {
  event EventZeroData();
  event EventOneData(uint a);
  event EventTwoData(uint a, uint b);
  event EventThreeData(uint a, uint b, uint c);
  event EventFourData(uint a, uint b, uint c, uint d);

  event EventOneIndexed(uint indexed a);
  event EventTwoIndexed(uint indexed a, uint indexed b);
  event EventThreeIndexed(uint indexed a, uint indexed b, uint indexed c);
 
  event EventOneIndexedWithData(uint indexed a, uint b);
  event EventTwoIndexedWithData(uint indexed a, uint indexed b, uint c);
  event EventThreeIndexedWithData(uint indexed a, uint indexed b, uint indexed c, uint d);
 
  function logEventZeroData() public {
    emit EventZeroData();
  }
  function logEventOneData(uint a) public {
    emit EventOneData(a);
  }
  function logEventTwoData(uint a, uint b) public {
    emit EventTwoData(a,b);
  }
  function logEventThreeData(uint a, uint b, uint c) public {
    emit EventThreeData(a,b,c);
  }
  function logEventFourData(uint a, uint b, uint c, uint d) public {
    emit EventFourData(a,b,c,d);
  }    
  function logEventOneIndexed(uint a) public {
    emit EventOneIndexed(a);
  }    
  function logEventTwoIndexed(uint a, uint b) public {
    emit EventTwoIndexed(a,b);
  }    
  function logEventThreeIndexed(uint a, uint b, uint c) public {
    emit EventThreeIndexed(a,b,c);
  }   
  function logEventOneIndexedWithData(uint a, uint b) public {
    emit EventOneIndexedWithData(a,b);
  }     
  function logEventTwoIndexedWithData(uint a, uint b, uint c) public {
    emit EventTwoIndexedWithData(a,b,c);
  }   
  function logEventThreeIndexedWithData(uint a, uint b, uint c, uint d) public {
    emit EventThreeIndexedWithData(a,b,c,d);
  }    
}

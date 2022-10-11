# Mir Validator
Application that implements the 

Use `make mir-validator` or `make buildernet` to compile the application. To
run a validator you need to:
- First spawn a lotus-daemon.
- Initialize the configuration of mir-validator in $LOTUS_PATH. 
`./mir-validator config init`
- The information about the validators membership for the network is currently stored
in `$LOTUS_PATH/mir.validators`. To add new validators to the membership list you can either
paste the information of the new validator in that file (with the format `<wallet_addr>@<multiaddr_with_/p2p_info>`), or run `./mir-validator config add-validator`.
- Run the validator process using `./mir-validator run --nosync`

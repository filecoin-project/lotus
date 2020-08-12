# Updating Lotus

If you installed Lotus on your machine, you can upgrade to the latest version by doing the following:

```sh
# get the latest
git pull origin master

# clean and remake the binaries
make clean && make build

# instal binaries in correct location
make install # or sudo make install if necessary
```

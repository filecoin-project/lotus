# Testground Composer

This is a work-in-progress UI for configuring and running testground compositions.

The app code lives in [./app](./app), and there's a thin Jupyter notebook shell in [composer.ipynb](./composer.ipynb).

## Running

You can either run the app in docker, or in a local python virtualenv. Docker is recommended unless you're hacking
on the code for Composer itself.

### Running with docker

Run the `./composer.sh` script to build a container with the latest source and run it. The first build
will take a little while since it needs to build testground and fetch a bunch of python dependencies.

You can skip the build if you set `SKIP_BUILD=true` when running `composer.sh`, and you can rebuild
manually with `make docker`.

The contents of `$TESTGROUND_HOME/plans` will be sync'd to a temporary directory and read-only mounted 
into the container.

After building and starting the container, the script will open a browser to the composer UI.

You should be able to load an existing composition or create a new one from one of the plans in 
`$TESTGROUND_HOME/plans`.

Right now docker only supports the standalone webapp UI; to run the UI in a Jupyter notebook, see below.

### Running with local python

To run without docker, make a python3 virtual environment somewhere and activate it:

```shell
# make a virtualenv called "venv" in the current directory
python3 -m venv ./venv

# activate (bash/zsh):
source ./venv/bin/activate

# activate (fish):
source ./venv/bin/activate.fish
```

Then install the python dependencies:

```shell
pip install -r requirements.txt
```

And start the UI:

```shell
panel serve composer.ipynb
```

That will start the standalone webapp UI. If you want a Jupyter notebook instead, run:

```
jupyter notebook
```

and open `composer.ipynb` in the Jupyter file picker.
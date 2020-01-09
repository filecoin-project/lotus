# Docker Instructions

To run lotus in Docker, first build the image: `docker build -t lotus .`

After building the image, any of the lotus binaries and commands are available to you. For example, run to run the daemon: `docker run -d -it -p 1234:1234 -p 1347:1347 lotus /home/lotus/app/lotus daemon`

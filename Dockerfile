FROM ubuntu:18.04

# 1. Create user
RUN useradd -ms /bin/bash lotus

# 2. Create the working directory at '/home/app' and give user use permissions
RUN mkdir -p /home/lotus/app && chown -R lotus:lotus /home/lotus/app

# 3. Set the working directory
WORKDIR /home/lotus/app

# 4. Install the deps
RUN apt-get update && \
    apt-get install -y software-properties-common && \
    apt-get update && \
    add-apt-repository -y ppa:longsleep/golang-backports && \
    apt-get update && \
    apt-get install -y mesa-opencl-icd ocl-icd-opencl-dev curl \
      golang-go gcc git bzr jq pkg-config mesa-opencl-icd ocl-icd-opencl-dev

# 5. More deps
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y

# 6. Copy the app
COPY --chown=lotus:lotus . .

# 7. Build
RUN make clean && make all

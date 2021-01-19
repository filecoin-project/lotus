FROM golang:1.15-buster as tg-build

ARG TESTGROUND_REF="oni"
WORKDIR /usr/src
RUN git clone https://github.com/testground/testground.git
RUN cd testground && git checkout $TESTGROUND_REF && go build .

FROM python:3.8-buster

WORKDIR /usr/src/app

COPY --from=tg-build /usr/src/testground/testground /usr/bin/testground

RUN mkdir /composer && chmod 777 /composer
RUN mkdir /testground && chmod 777 /testground

ENV HOME /composer
ENV TESTGROUND_HOME /testground
ENV LISTEN_PORT 5006
ENV TESTGROUND_DAEMON_HOST host.docker.internal

VOLUME /testground/plans


COPY requirements.txt ./
RUN pip install -r requirements.txt
COPY . .

CMD panel serve --address 0.0.0.0 --port $LISTEN_PORT composer.ipynb

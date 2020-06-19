FROM python:3.8-buster

WORKDIR /usr/src/app

RUN mkdir /composer && chmod 777 /composer
RUN mkdir /testground && chmod 777 /testground

ENV HOME /composer
ENV TESTGROUND_HOME /testground
ENV LISTEN_PORT 5006

VOLUME /testground/plans


COPY requirements.txt ./
RUN pip install -r requirements.txt
COPY . .

CMD panel serve --address 0.0.0.0 --port $LISTEN_PORT composer.ipynb
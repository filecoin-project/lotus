FROM python:3.8-buster

WORKDIR /usr/src/app

RUN mkdir /jupyter && chmod 777 /jupyter

COPY requirements.txt ./
RUN pip install -r requirements.txt

COPY . .

VOLUME /testground/plans
VOLUME /jupyter/runtime

ENV TESTGROUND_HOME /testground
ENV HOME /jupyter
ENV JUPYTER_RUNTIME_DIR /jupyter/runtime

CMD ["jupyter", "notebook", "--allow-root", "--ip", "0.0.0.0"]
FROM centos:centos8
RUN cd /etc/yum.repos.d/
RUN sed -i 's/mirrorlist/#mirrorlist/g' /etc/yum.repos.d/CentOS-*
RUN sed -i 's|#baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|g' /etc/yum.repos.d/CentOS-*
RUN yum upgrade -y
RUN yum install procps-ng wget libatomic python39 -y
RUN alternatives --set python /usr/bin/python3
RUN arch=$(arch | sed s/aarch64/el8-aarch64/ | sed s/x86_64/linux-x86_64/) && wget "https://downloads.yugabyte.com/releases/2.20.2.0/yugabyte-2.20.2.0-b145-${arch}.tar.gz" -O /tmp/yugabyte.tar.gz
RUN tar xvfz /tmp/yugabyte.tar.gz
RUN ln -s /yugabyte-2.20.2.0 /yugabyte
RUN /yugabyte/bin/post_install.sh
CMD /yugabyte/bin/yugabyted start --daemon=false --ui=false

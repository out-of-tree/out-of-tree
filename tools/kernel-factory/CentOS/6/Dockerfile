FROM centos:6

RUN yum -y groupinstall "Development Tools"

# enable rpms from old minor releases
RUN sed -i 's/enabled=0/enabled=1/' /etc/yum.repos.d/CentOS-Vault.repo

# do not remove old kernels
RUN sed -i 's;installonly_limit=;installonly_limit=100500;' /etc/yum.conf

RUN yum list --showduplicates kernel | grep kernel | awk '{print $2}' \
            | while read VERSION; do \
                    echo kernel-devel-${VERSION} kernel-${VERSION}; \
            done | xargs yum -y install

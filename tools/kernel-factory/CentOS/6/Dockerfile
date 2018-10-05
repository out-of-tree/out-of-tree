FROM centos:6

RUN yum -y groupinstall "Development Tools"

# Last release must be download from mirror.centos.org, because vault.centos.org
# does not containt repodata for last release.
RUN yum list --showduplicates kernel \
	| grep kernel | awk '{print $2}' | while read VERSION; do \
			echo kernel-devel-${VERSION} kernel-${VERSION}; \
		done | xargs yum install -y;

# mirror.centos.org does not contain old releases.
RUN sed -i 's/^mirrorlist/#mirrorlist/' /etc/yum.repos.d/*
RUN sed -i 's;^#baseurl=http://mirror.centos.org/centos/;baseurl=http://vault.centos.org/;' /etc/yum.repos.d/*

# do not remove old kernels
RUN sed -i 's;installonly_limit=;installonly_limit=100500;' /etc/yum.conf

# After new release must put PREVIOUS version here, e.g. at the time of
# writing the comment last CentOS version was 6.10.
RUN for RELEASEVER in 6.0 6.1 6.2 6.3 6.4 6.5 6.6 6.7 6.8 6.9; do \
	yum list --releasever=${RELEASEVER} --showduplicates kernel \
		| grep kernel | awk '{print $2}' | while read VERSION; do \
				echo kernel-devel-${VERSION} kernel-${VERSION}; \
			done | xargs yum install --releasever=${RELEASEVER} -y; \
	done

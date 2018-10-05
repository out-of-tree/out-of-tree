FROM centos:7

# Last release must be download from mirror.centos.org, because vault.centos.org
# does not containt repodata for last release.
RUN yum list --showduplicates kernel \
	| grep kernel | awk '{print $2}' | while read VERSION; do \
			echo kernel-devel-${VERSION} kernel-${VERSION}; \
		done | xargs yum install -y;

# mirror.centos.org does not contain old releases.
RUN sed -i 's/^mirrorlist/#mirrorlist/' /etc/yum.repos.d/*
RUN sed -i 's;^#baseurl=http://mirror;baseurl=http://vault;' /etc/yum.repos.d/*

# do not remove old kernels
RUN sed -i 's;installonly_limit=;installonly_limit=100500;' /etc/yum.conf

# After new release must put PREVIOUS version here, e.g. at the time of
# writing the comment last CentOS version was 7.5.1804.
RUN for RELEASEVER in 7.0.1406 7.1.1503 7.2.1511 7.3.1611 7.4.1708; do \
	yum list --releasever=${RELEASEVER} --showduplicates kernel \
		| grep kernel | awk '{print $2}' | while read VERSION; do \
				echo kernel-devel-${VERSION} kernel-${VERSION}; \
			done | xargs yum install --releasever=${RELEASEVER} -y; \
	done

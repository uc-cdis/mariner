FROM ubuntu:16.04

RUN apt-get update \
	&& apt-get install -y \
		bzip2 \
		gcc \
		libncurses-dev \
		make \
		wget \
		zlib1g-dev \
		libbz2-dev \
		liblzma-dev \
	&& apt-get clean \
    && cd /opt/ && wget https://github.com/samtools/samtools/releases/download/1.9/samtools-1.9.tar.bz2 \
    && tar xf samtools-1.9.tar.bz2 \
    && cd samtools-1.9 \
    && make \
    && make install \
    && cd - \
    && rm -rf samtools-1.9.tar.bz2 \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

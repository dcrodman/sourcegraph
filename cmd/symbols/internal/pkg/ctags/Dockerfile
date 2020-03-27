FROM alpine:3.10@sha256:de78803598bc4c940fc4591d412bffe488205d5d953f94751c6308deeaaa7eb8 AS ctags

ENV CTAGS_VERSION=03f933a96d3ef87adbf9d167462d45ce69577edb

# hadolint ignore=DL3003,DL3018,DL4006
RUN apk --no-cache add --virtual build-deps curl jansson-dev libseccomp-dev linux-headers autoconf pkgconfig make automake gcc g++ binutils && \
  curl https://codeload.github.com/universal-ctags/ctags/tar.gz/$CTAGS_VERSION | tar xz -C /tmp && \
  cd /tmp/ctags-$CTAGS_VERSION && \
  ./autogen.sh && \
  LDFLAGS=-static ./configure --program-prefix=universal- --enable-json --enable-seccomp && \
  make -j8 && \
  make install && \
  cd && \
  rm -rf /tmp/ctags-$CTAGS_VERSION && \
  apk --no-cache --purge del build-deps

WORKDIR /
COPY .ctags.d /.ctags.d

#!/bin/bash
set -e

cd /tmp

wget "https://github.com/libgit2/libgit2/archive/v$LIBGIT2_VERSION.tar.gz" -O libgit2.tar.gz
tar xzf libgit2.tar.gz
cd "libgit2-$LIBGIT2_VERSION"

cmake -DCMAKE_INSTALL_PREFIX=/tmp/libgit2
cmake --build . --target install

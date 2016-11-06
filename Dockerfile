FROM alpine:3.4
MAINTAINER Minecrell <dev@minecrell.net>
EXPOSE 4000

CMD ["SpongeDownloads"]
ENV GIT_STORAGE_DIR /tmp/git

COPY . /go/src/github.com/Minecrell/SpongeDownloads

RUN apk add --no-cache libgit2 \
    && apk add --no-cache --virtual build-deps go gcc git musl-dev libgit2-dev \
    && export GOPATH=/go \
    && go get -v github.com/Minecrell/SpongeDownloads \
    && apk del build-deps \
    && cp $GOPATH/bin/SpongeDownloads /usr/bin \
    && rm -rf $GOPATH \
    && adduser -S go

USER go

#
# MIT License
#
# (C) Copyright 2021-2022 Hewlett Packard Enterprise Development LP
#
# Permission is hereby granted, free of charge, to any person obtaining a
# copy of this software and associated documentation files (the "Software"),
# to deal in the Software without restriction, including without limitation
# the rights to use, copy, modify, merge, publish, distribute, sublicense,
# and/or sell copies of the Software, and to permit persons to whom the
# Software is furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included
# in all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
# THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
# OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
# ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
# OTHER DEALINGS IN THE SOFTWARE.
#
# Build base just has the packages installed we need.
FROM artifactory.algol60.net/docker.io/library/golang:1.19-alpine AS build-base

RUN set -ex \
    && apk update \
    && apk add --no-cache \
        build-base \
        git

# Base copies in the files we need to test/build.
FROM build-base AS base

WORKDIR /build

# Copy all the necessary files to the image.
COPY cmd        cmd
COPY internal   internal
#COPY pkg     pkg
COPY vendor  vendor

# Copy the Go module files.
COPY go.mod .
COPY go.sum .

### Build Stage ###
FROM base AS builder

ARG go_build_args="-mod=vendor"

RUN set -ex \
    && go build ${go_build_args} -v -o /usr/local/bin/cray-powerdns-manager ./cmd/manager \
    && go build ${go_build_args} -v -o /usr/local/bin/cray-externaldns-manager ./cmd/externaldns-manager \
    && go build ${go_build_args} -v -o /usr/local/bin/cray-powerdns-visualizer ./cmd/visualizer

## Final Stage ###
FROM artifactory.algol60.net/csm-docker/stable/docker.io/library/alpine:3
LABEL maintainer="Cray, Inc."

COPY --from=builder /usr/local/bin/cray-powerdns-manager /usr/local/bin
COPY --from=builder /usr/local/bin/cray-externaldns-manager /usr/local/bin
COPY --from=builder /usr/local/bin/cray-powerdns-visualizer /usr/local/bin

COPY .version /.version

RUN set -ex \
    && apk update \
    && apk add --no-cache curl

CMD ["sh", "-c", "cray-powerdns-manager"]

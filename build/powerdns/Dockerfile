FROM ubuntu:focal AS build-base

RUN set -ex \
    && apt update \
    && apt-get -y upgrade \
    && DEBIAN_FRONTEND=noninteractive apt-get -y install \
        curl \
        gnupg2 \
    && curl https://repo.powerdns.com/FD380FBB-pub.asc | apt-key add -


FROM build-base AS base

# Copy the files FIRST so we prefer the PowerDNS repos instead of Ubuntu's.
COPY files /

# Install the necessary packages.
RUN set -ex \
    && apt update \
    && DEBIAN_FRONTEND=noninteractive apt-get -y install \
        pdns-server \
        pdns-backend-sqlite3 \
        sqlite3

FROM base

EXPOSE 80/tcp
EXPOSE 53/udp
EXPOSE 53/tcp

COPY entrypoint.sh /

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/usr/sbin/pdns_server"]
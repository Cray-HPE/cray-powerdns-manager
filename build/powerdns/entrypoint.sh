#!/usr/bin/env bash

rm -f /etc/powerdns/pdns.d/bind.conf

exec "$@"
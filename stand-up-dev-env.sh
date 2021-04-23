#!/usr/bin/env bash

set -ex

docker-compose -f docker-compose.devel.yaml down
docker-compose -f docker-compose.devel.yaml up -d

# Add the supermasters record to the slave PowerDNS.
docker exec -i -t cray-powerdns-manager_secondary-mariadb_1 mysql -u root --password=root powerdns \
-e "insert into supermasters values ('192.168.53.4', 'ns2.shasta.dev.cray.com', 'admin');"
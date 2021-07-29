#!/usr/bin/env bash

SYSTEM_NAME="${SYSTEM_NAME:-shasta.dev.cray.com}"

function add_supermaster() {
  docker exec -i -t cray-powerdns-manager_secondary-mariadb_1 mysql -u root --password=root powerdns \
    -e "insert into supermasters values ('192.168.53.4', 'primary.$SYSTEM_NAME', 'admin');" > /dev/null

  return $?
}

set -e

docker-compose -f docker-compose.devel.yaml down
docker-compose -f docker-compose.devel.yaml up -d

until add_supermaster
do
  echo "Waiting for MySQL to be ready..."
done

echo "Done!"
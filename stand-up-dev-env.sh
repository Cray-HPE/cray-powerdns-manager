#!/usr/bin/env bash

SYSTEM_NAME="${SYSTEM_NAME:-shasta.dev.cray.com}"

function seed_database() {
  docker exec -i -t cray-powerdns-manager_secondary-mariadb_1 mysql -u root --password=root powerdns \
    -e "insert into supermasters values ('192.168.53.4', 'primary.$SYSTEM_NAME', 'admin');" > /dev/null

  docker exec -i -t cray-powerdns-manager_secondary-mariadb_1 mysql -u root --password=root powerdns \
    -e "insert into tsigkeys (name, algorithm, secret) values
    ('test-key', 'hmac-sha256', 'dnFC5euKixIKXAr6sZhI7kVQbQCXoDG5R5eHSYZiBxY=');" > /dev/null

  return $?
}

set -e

docker-compose -f docker-compose.devel.yaml down
docker-compose -f docker-compose.devel.yaml up -d

until seed_database
do
  echo "Waiting for MySQL to be ready..."
done

echo "Done!"
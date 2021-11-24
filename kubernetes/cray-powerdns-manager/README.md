# cray-powerdns-manager

![Version: 0.5.2](https://img.shields.io/badge/Version-0.5.2-informational?style=flat-square) ![AppVersion: 0.5.2](https://img.shields.io/badge/AppVersion-0.5.2-informational?style=flat-square)

Synchronizes all DNS records for Cray EX systems

**Homepage:** <https://github.com/Cray-HPE/cray-powerdns-manager>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| dle-hpe |  |  |
| SeanWallace |  |  |
| spillerc-hpe |  |  |

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://artifactory.algol60.net/artifactory/csm-helm-charts/ | cray-service | ~7.0.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| cray-service.containers.cray-powerdns-manager.env[0].name | string | `"BASE_DOMAIN"` |  |
| cray-service.containers.cray-powerdns-manager.env[0].valueFrom.configMapKeyRef.key | string | `"base_domain"` |  |
| cray-service.containers.cray-powerdns-manager.env[0].valueFrom.configMapKeyRef.name | string | `"cray-powerdns-manager-config"` |  |
| cray-service.containers.cray-powerdns-manager.env[1].name | string | `"PRIMARY_SERVER"` |  |
| cray-service.containers.cray-powerdns-manager.env[1].valueFrom.configMapKeyRef.key | string | `"primary_server"` |  |
| cray-service.containers.cray-powerdns-manager.env[1].valueFrom.configMapKeyRef.name | string | `"cray-powerdns-manager-config"` |  |
| cray-service.containers.cray-powerdns-manager.env[2].name | string | `"SECONDARY_SERVERS"` |  |
| cray-service.containers.cray-powerdns-manager.env[2].valueFrom.configMapKeyRef.key | string | `"secondary_servers"` |  |
| cray-service.containers.cray-powerdns-manager.env[2].valueFrom.configMapKeyRef.name | string | `"cray-powerdns-manager-config"` |  |
| cray-service.containers.cray-powerdns-manager.env[3].name | string | `"NOTIFY_ZONES"` |  |
| cray-service.containers.cray-powerdns-manager.env[3].valueFrom.configMapKeyRef.key | string | `"notify_zones"` |  |
| cray-service.containers.cray-powerdns-manager.env[3].valueFrom.configMapKeyRef.name | string | `"cray-powerdns-manager-config"` |  |
| cray-service.containers.cray-powerdns-manager.env[4].name | string | `"PDNS_URL"` |  |
| cray-service.containers.cray-powerdns-manager.env[4].value | string | `"http://cray-dns-powerdns-api:8081"` |  |
| cray-service.containers.cray-powerdns-manager.env[5].name | string | `"PDNS_API_KEY"` |  |
| cray-service.containers.cray-powerdns-manager.env[5].valueFrom.secretKeyRef.key | string | `"pdns_api_key"` |  |
| cray-service.containers.cray-powerdns-manager.env[5].valueFrom.secretKeyRef.name | string | `"cray-powerdns-credentials"` |  |
| cray-service.containers.cray-powerdns-manager.env[6].name | string | `"KEY_DIRECTORY"` |  |
| cray-service.containers.cray-powerdns-manager.env[6].value | string | `"/keys"` |  |
| cray-service.containers.cray-powerdns-manager.image.pullPolicy | string | `"Always"` |  |
| cray-service.containers.cray-powerdns-manager.image.repository | string | `"artifactory.algol60.net/csm-docker/stable/cray-powerdns-manager"` |  |
| cray-service.containers.cray-powerdns-manager.livenessProbe.httpGet.path | string | `"/v1/liveness"` |  |
| cray-service.containers.cray-powerdns-manager.livenessProbe.httpGet.port | int | `8080` |  |
| cray-service.containers.cray-powerdns-manager.livenessProbe.initialDelaySeconds | int | `10` |  |
| cray-service.containers.cray-powerdns-manager.livenessProbe.periodSeconds | int | `30` |  |
| cray-service.containers.cray-powerdns-manager.name | string | `"cray-powerdns-manager"` |  |
| cray-service.containers.cray-powerdns-manager.ports[0].containerPort | int | `8080` |  |
| cray-service.containers.cray-powerdns-manager.ports[0].name | string | `"http"` |  |
| cray-service.containers.cray-powerdns-manager.readinessProbe.httpGet.path | string | `"/v1/readiness"` |  |
| cray-service.containers.cray-powerdns-manager.readinessProbe.httpGet.port | int | `8080` |  |
| cray-service.containers.cray-powerdns-manager.readinessProbe.initialDelaySeconds | int | `15` |  |
| cray-service.containers.cray-powerdns-manager.readinessProbe.periodSeconds | int | `30` |  |
| cray-service.containers.cray-powerdns-manager.volumeMounts[0].mountPath | string | `"/keys"` |  |
| cray-service.containers.cray-powerdns-manager.volumeMounts[0].name | string | `"dnssec-keys"` |  |
| cray-service.containers.cray-powerdns-manager.volumeMounts[0].readOnly | bool | `true` |  |
| cray-service.fullnameOverride | string | `"cray-powerdns-manager"` |  |
| cray-service.ingress.enabled | bool | `true` |  |
| cray-service.ingress.prefix | string | `"/apis/powerdns-manager/"` |  |
| cray-service.ingress.uri | string | `"/"` |  |
| cray-service.nameOverride | string | `"cray-powerdns-manager"` |  |
| cray-service.priorityClassName | string | `"csm-high-priority-service"` |  |
| cray-service.replicaCount | int | `1` |  |
| cray-service.serviceAccountName | string | `"jobs-watcher"` |  |
| cray-service.strategy.type | string | `"Recreate"` |  |
| cray-service.type | string | `"Deployment"` |  |
| global.appVersion | string | `"0.5.2"` |  |
| global.chart.name | string | `"cray-powerdns-manager"` |  |
| global.chart.version | string | `"0.5.2"` |  |
| manager.notify_zones | string | `""` |  |
| manager.primary_server | string | `""` |  |
| manager.secondary_servers | string | `""` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.5.0](https://github.com/norwoodj/helm-docs/releases/v1.5.0)

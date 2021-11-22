NAME ?= cray-powerdns-manager
CHART_PATH ?= kubernetes
VERSION ?= $(shell cat .version)-local
CHART_VERSION ?= $(VERSION)

HELM_UNITTEST_IMAGE ?= quintush/helm-unittest:3.3.0-0.2.5

all : image chart
chart: chart_setup chart_package chart_test

image:
	docker build --pull ${DOCKER_ARGS} --tag '${NAME}:${VERSION}' .

chart_setup:
	mkdir -p ${CHART_PATH}/.packaged
	docker run --rm -v ${PWD}:/apps artifactory.algol60.net/docker.io/library/bash bash -c 'wget https://github.com/mikefarah/yq/releases/download/v4.14.2/yq_linux_amd64 -O /usr/bin/yq && chmod +x /usr/bin/yq && /apps/hack/update-chart-globals.sh /apps/${CHART_PATH}/${NAME}'

chart_package:
	helm dep up ${CHART_PATH}/${NAME}
	helm package ${CHART_PATH}/${NAME} -d ${CHART_PATH}/.packaged --app-version ${VERSION} --version ${CHART_VERSION}

chart_test:
	helm lint "${CHART_PATH}/${NAME}"
	docker run --rm -v ${PWD}/${CHART_PATH}:/apps ${HELM_UNITTEST_IMAGE} -3 ${NAME}

clean:
	$(RM) -r ${CHART_PATH}/.packaged

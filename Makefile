NAME ?= cray-powerdns-manager
VERSION ?= $(shell cat .version)

CHART_VERSION ?= $(VERSION)
IMAGE ?= artifactory.algol60.net/csm-docker/stable/${NAME}

CHARTDIR ?= kubernetes

CHART_METADATA_IMAGE ?= artifactory.algol60.net/csm-docker/stable/chart-metadata
YQ_IMAGE ?= artifactory.algol60.net/docker.io/mikefarah/yq:4
HELM_IMAGE ?= artifactory.algol60.net/docker.io/alpine/helm:3.7.1
HELM_UNITTEST_IMAGE ?= artifactory.algol60.net/docker.io/quintush/helm-unittest
HELM_DOCS_IMAGE ?= artifactory.algol60.net/docker.io/jnorwood/helm-docs:v1.5.0

all : image chart

image:
	docker build --no-cache --pull ${DOCKER_ARGS} --tag '${NAME}:${VERSION}' .

chart: chart_metadata chart_package chart_test

chart_metadata:
	docker run --rm \
		--user $(shell id -u):$(shell id -g) \
		-v ${PWD}/${CHARTDIR}/${NAME}:/chart \
		${CHART_METADATA_IMAGE} \
		--version "${CHART_VERSION}" --app-version "${VERSION}" \
		-i ${NAME} ${IMAGE}:${VERSION} \
		--cray-service-globals
	docker run --rm \
		--user $(shell id -u):$(shell id -g) \
		-v ${PWD}/${CHARTDIR}/${NAME}:/chart \
		-w /chart \
		${YQ_IMAGE} \
		eval -Pi '.cray-service.containers.${NAME}.image.repository = "${IMAGE}"' values.yaml

helm:
	docker run --rm \
	    --user $(shell id -u):$(shell id -g) \
	    --mount type=bind,src="$(shell pwd)",dst=/src \
	    -w /src \
	    -e HELM_CACHE_HOME=/src/.helm/cache \
	    -e HELM_CONFIG_HOME=/src/.helm/config \
	    -e HELM_DATA_HOME=/src/.helm/data \
	    $(HELM_IMAGE) \
	    $(CMD)

chart_package: ${CHARTDIR}/.packaged/${NAME}-${CHART_VERSION}.tgz

chart_test:
	CMD="lint ${CHARTDIR}/${NAME}" $(MAKE) helm
	docker run --rm -v ${PWD}/${CHARTDIR}:/apps ${HELM_UNITTEST_IMAGE} -3 ${NAME}

${CHARTDIR}/.packaged/${NAME}-${CHART_VERSION}.tgz: ${CHARTDIR}/.packaged
	CMD="dep up ${CHARTDIR}/${NAME}" $(MAKE) helm
	CMD="package ${CHARTDIR}/${NAME} -d ${CHARTDIR}/.packaged" $(MAKE) helm

${CHARTDIR}/.packaged:
	mkdir -p ${CHARTDIR}/.packaged

clean:
	$(RM) -r ${CHARTDIR}/.packaged .helm

chart_images: ${CHARTDIR}/.packaged/${NAME}-${CHART_VERSION}.tgz
	{ CMD="template release $< --dry-run --replace --dependency-update --set manager.base_domain=example.com" $(MAKE) -s helm; \
	  echo '---' ; \
	  CMD="show chart $<" $(MAKE) -s helm | docker run --rm -i $(YQ_IMAGE) e -N '.annotations."artifacthub.io/images"' - ; \
	} | docker run --rm -i $(YQ_IMAGE) e -N '.. | .image? | select(.)' - | sort -u

gen-docs:
	docker run --rm \
	    --user $(shell id -u):$(shell id -g) \
	    --mount type=bind,src="$(shell pwd)",dst=/src \
	    -w /src \
	    $(HELM_DOCS_IMAGE) \
	    helm-docs

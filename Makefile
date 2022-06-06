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

chart: chart-metadata chart-package chart-test

chart-metadata:
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

chart-package: ${CHARTDIR}/.packaged/${NAME}-${CHART_VERSION}.tgz

${CHARTDIR}/.packaged/${NAME}-${CHART_VERSION}.tgz: ${CHARTDIR}/.packaged
	CMD="dep up ${CHARTDIR}/${NAME}" $(MAKE) helm
	sed -e '/^  base_domain: example.com /d' -i ${CHARTDIR}/${NAME}/values.yaml
	CMD="package ${CHARTDIR}/${NAME} -d ${CHARTDIR}/.packaged" $(MAKE) helm
	git restore ${CHARTDIR}/${NAME}/values.yaml

${CHARTDIR}/.packaged:
	mkdir -p ${CHARTDIR}/.packaged

chart-test:
	CMD="lint ${CHARTDIR}/${NAME}" $(MAKE) helm
	docker run --rm -v ${PWD}/${CHARTDIR}:/apps ${HELM_UNITTEST_IMAGE} -3 ${NAME}

chart-images: ${CHARTDIR}/.packaged/${NAME}-${CHART_VERSION}.tgz
	{ CMD="template release $< --dry-run --replace --dependency-update --set manager.base_domain=example.com" $(MAKE) -s helm; \
	  echo '---' ; \
	  CMD="show chart $<" $(MAKE) -s helm | docker run --rm -i $(YQ_IMAGE) e -N '.annotations."artifacthub.io/images"' - ; \
	} | docker run --rm -i $(YQ_IMAGE) e -N '.. | .image? | select(.)' - | sort -u

snyk:
	$(MAKE) -s chart-images | xargs --verbose -n 1 snyk container test

chart-gen-docs:
	docker run --rm \
	    --user $(shell id -u):$(shell id -g) \
	    --mount type=bind,src="$(shell pwd)",dst=/src \
	    -w /src \
	    $(HELM_DOCS_IMAGE) \
	    helm-docs --chart-search-root=$(CHARTDIR)

clean:
	$(RM) -r ${CHARTDIR}/.packaged .helm

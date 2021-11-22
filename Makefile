NAME ?= cray-powerdns-manager
VERSION ?= $(shell cat .version)-local

CHARTDIR ?= kubernetes

YQ_IMAGE ?= artifactory.algol60.net/docker.io/mikefarah/yq:4
HELM_UNITTEST_IMAGE ?= artifactory.algol60.net/docker.io/quintush/helm-unittest

# Get image repository
IMAGE_REPOSITORY ?= $(shell docker run --rm -i ${YQ_IMAGE} e '.cray-service.containers.${NAME}.image.repository' - < ${CHARTDIR}/${NAME}/values.yaml)

# Define chart annotations
define ANNOTATION_IMAGES
- name: ${NAME}
  image: ${IMAGE_REPOSITORY}:${VERSION}
endef
export ANNOTATION_IMAGES

all : image chart
chart: chart_metadata chart_package chart_test

.PHONY: image chart_metadata chart_test clean

image:
	docker build --pull ${DOCKER_ARGS} --tag '${NAME}:${VERSION}' .

chart_metadata:
	docker run --rm -v ${PWD}/${CHARTDIR}/${NAME}:/chart ${YQ_IMAGE} e -Pi ".version = \"${VERSION}\", .appVersion = \"${VERSION}\", .annotations.\"artifacthub.io/images\" = \"$${ANNOTATION_IMAGES}\"" /chart/Chart.yaml
	docker run --rm -v ${PWD}/${CHARTDIR}/${NAME}:/chart ${YQ_IMAGE} e -Pi '.global.chart.name = "${NAME}", .global.chart.version = "${VERSION}", .global.appVersion = "${VERSION}"' /chart/values.yaml

chart_package: ${CHARTDIR}/.packaged/${NAME}-${VERSION}.tgz

chart_test:
	helm lint ${CHARTDIR}/${NAME}
	docker run --rm -v ${PWD}/${CHARTDIR}:/apps ${HELM_UNITTEST_IMAGE} -3 ${NAME}

clean:
	$(RM) -r ${CHARTDIR}/.packaged

${CHARTDIR}/.packaged/${NAME}-${VERSION}.tgz: ${CHARTDIR}/.packaged
	helm dep up ${CHARTDIR}/${NAME}
	helm package ${CHARTDIR}/${NAME} -d ${CHARTDIR}/.packaged --app-version ${VERSION} --version ${VERSION}

${CHARTDIR}/.packaged:
	mkdir -p ${CHARTDIR}/.packaged

chart_images: ${CHARTDIR}/.packaged/${NAME}-${VERSION}.tgz
	@{ \
	  helm template release $< --dry-run --replace --dependency-update --set manager.base_domain=example.com; \
	  echo '---' ; \
	  helm show chart $< | docker run --rm -i artifactory.algol60.net/docker.io/mikefarah/yq:4 e -N '.annotations."artifacthub.io/images"' - ; \
	} | docker run --rm -i artifactory.algol60.net/docker.io/mikefarah/yq:4 e -N '.. | .image? | select(.)' - | sort -u


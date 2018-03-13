#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

version=`cat VERSION`

TAG="${version}" make dockerize
docker tag "projectriff/go-function-invoker:latest" "projectriff/go-function-invoker:${version}-ci-${TRAVIS_COMMIT}"

docker login -u "${DOCKER_USERNAME}" -p "${DOCKER_PASSWORD}"
docker push "projectriff/go-function-invoker"

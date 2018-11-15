#!/usr/bin/env bash
#
# Code coverage generation
#set -e -o pipefail

echo "TRAVIS_BRANCH=$TRAVIS_BRANCH"
echo "TRAVIS_PULL_REQUEST_BRANCH=$TRAVIS_PULL_REQUEST_BRANCH"
echo "TRAVIS_PULL_REQUEST=$TRAVIS_PULL_REQUEST"
echo "TRAVIS_PULL_REQUEST_SHA=$TRAVIS_PULL_REQUEST_SHA"
echo "TRAVIS_PULL_REQUEST_SLUG=$TRAVIS_PULL_REQUEST_SLUG"
echo "TRAVIS_REPO_SLUG=$TRAVIS_REPO_SLUG"
echo "TRAVIS_BUILD_DIR=$TRAVIS_BUILD_DIR"
echo "cur dir"
pwd
basepath=$(cd `dirname $0`; pwd)
echo "base=$basepath"
echo "git branch"
git branch
echo "git status"
git status






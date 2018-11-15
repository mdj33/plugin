#!/usr/bin/env bash
#
# Code coverage generation
#set -e -o pipefail
set -v


setup_git() {
    git config --global user.email "travis@travis-ci.org"
    git config --global user.name "mdj33"
}

commit_website_files() {
#    git checkout -b gh-pages
    git status
    git add .
    git status
    git commit --message "Travis build"
    git status
#    git push origin HEAD:$TRAVIS_BRANCH
}

upload_files() {
    local token="60b06b3dbdd5b5e2ce997a103add655d4a3686e0"
#    curl -H 'Authorization: token <e32f4a9bcfc918e8e1d4928fa47704d3eb451100>'  https://github.com/mdj33/plugin.git
#    git remote rm origin
    git remote add originx https://"${token}"@github.com/mdj33/plugin.git >/dev/null 2>&1
#    git remote -v
#    git branch -r
    git push --quiet --set-upstream originx add_autoci
}


setup_git
commit_website_files
upload_files


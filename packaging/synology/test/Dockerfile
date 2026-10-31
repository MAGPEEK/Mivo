# Image of the package script tests (README.md in this directory): bats-core with bats-support
# and bats-assert, plus dash for the default shell matrix, jq for the JSON checks and Node.js,
# which evaluates the wizard's share-name regex with the JavaScript engine a browser uses.
#
#   docker build -t spk-script-test:dev packaging/synology/test
FROM bats/bats:1.14.0
RUN apk add --no-cache dash jq nodejs

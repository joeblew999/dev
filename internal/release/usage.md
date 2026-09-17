### Releasing

A release is built for every platform, signed, and published to GitHub. The
same thing happens on your machine and in CI, from the same task, so a
release you can make locally is one CI can make too.

Consumers pin the version and the public key it was signed with, and get a
verified download plus the command's own manual with it.

Needs goreleaser, packslip and gh, and a clean tree to publish.

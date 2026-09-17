### Releasing

- `dev release DIR [VERSION] [--snapshot] [--name NAME]`
  publish a GitHub Release of the command in DIR, the same locally and in
  GitHub Actions: VERSION here (vX.Y.Z), the pushed tag there. Build every
  platform with goreleaser, sign the packslip manifest, upload. NAME is the
  binary's name; default the repo's. Every directory under `skills/` ships as
  a skill. `--snapshot` builds, signs with a throwaway key and verifies,
  publishing nothing; check runs it.
- `dev release DIR --keygen [--rotate]`
  make the signing key every release is signed with: the private half into
  fnox (`PACKSLIP_SIGNING_KEY`) and the repo's Actions secrets, the public
  half into `packslip.pub` for consumers to pin as their `pubkey`. With
  `--rotate`, replace a key that already exists and say what every consumer
  must do; without it, an existing key is left alone.

Needs goreleaser, packslip and gh, and a clean tree to publish.

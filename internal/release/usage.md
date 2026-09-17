### Releasing

- `dev release DIR [VERSION] [--snapshot] [--name NAME]`
  publish a GitHub Release of the command in DIR, the same locally and in
  GitHub Actions: VERSION here (vX.Y.Z), the pushed tag there. Build every
  platform with goreleaser, sign the packslip manifest, upload. Signed with
  the key in fnox (`PACKSLIP_SIGNING_KEY`), which `--keygen` makes once, with
  its public half in `packslip.pub` for consumers to pin as their `pubkey`.
  `--snapshot` builds, signs with a throwaway key and verifies, publishing
  nothing; check runs it. NAME is the binary's name; default the repo's.
  Every directory under `skills/` ships as a skill.

Needs goreleaser, packslip and gh, and a clean tree to publish.

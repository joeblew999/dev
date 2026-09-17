### Secrets

- `dev secrets set DIR NAME|OWNER [--names LIST] [--generate] [--if-missing] [--env NAME]`
  store a secret in fnox and push it to the app in DIR; `--generate` makes a
  random value instead of prompting, `--if-missing` leaves an existing one
  alone. With `--names`, the project's `NAME<TAB>OWNER` lines, an owner such as
  a provider name resolves to its secret
- `dev secrets ci NAME...`
  give the repo's GitHub Actions each named secret from fnox (gh secret set,
  the value on stdin), for what CI must do with a credential: sign a
  release with the shared key, deploy to Fly as upstream's workflow does
- `dev secrets push DIR [--env NAME] [--fix TEMPLATE]`
  read `NAME<TAB>OWNER` lines on stdin and push each secret from fnox to the
  app in DIR; a missing one prints TEMPLATE with `{provider}` filled in, and
  any problem makes the exit code 1

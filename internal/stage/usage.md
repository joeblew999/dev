### Stages

Every command goes through the same stages, whatever it is made of. A stage
reads the directory to decide what applies — a Go main, a package.json, gsx
sources, a wrangler.toml — so the same verb works on a plain command, a
Worker and a UI without being told which it is.

Run them through their mise tasks rather than by hand, so what you run
locally is what CI runs.

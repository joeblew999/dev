// Package conf reads a repo's TOML: wrangler.toml, fly.toml, session.toml,
// and whatever a stage inspects. One decode, one error wording, one place to
// change when a file must be read differently — five packages had written the
// same three lines and worded the failure five ways.
package conf

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// Load decodes path into a T. The type comes from the caller, so a config is
// a struct where it is used rather than a map passed around.
func Load[T any](path string) (T, error) {
	v, _, err := LoadStrict[T](path)
	return v, err
}

// LoadStrict is Load, and also every key the struct did not claim. A file
// that decides something — which skills a session pins — reads with this, so
// a typo fails rather than silently dropping what it names.
func LoadStrict[T any](path string) (T, []string, error) {
	var v T
	meta, err := toml.DecodeFile(path, &v)
	if err != nil {
		return v, nil, fmt.Errorf("%s: %w", path, err)
	}
	return v, undecodedKeys(meta), nil
}

// undecodedKeys is toml's own list, as strings.
func undecodedKeys(meta toml.MetaData) []string {
	keys := meta.Undecoded()
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.String())
	}
	return out
}

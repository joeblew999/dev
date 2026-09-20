package cloudflare

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joeblew999/dev/cli"
	"github.com/joeblew999/dev/internal/conf"
	"github.com/joeblew999/dev/internal/suffix"
)

const (
	// Where a developer's own workers.dev subdomain is remembered, so the
	// account is asked for it once rather than on every url.
	localFile    = "mise.local.toml"
	subdomainKey = "CLOUDFLARE_WORKERS_SUBDOMAIN"
)

var subdomainEndpoint = "https://api.cloudflare.com/client/v4/accounts/%s/workers/subdomain"

// wranglerConfig is the little of wrangler.toml a URL needs.
type wranglerConfig struct {
	Name string      `toml:"name"`
	KV   []kvBinding `toml:"kv_namespaces"`
	Env  map[string]struct {
		Name string      `toml:"name"`
		KV   []kvBinding `toml:"kv_namespaces"`
	} `toml:"env"`
}

type kvBinding struct {
	Binding string `toml:"binding"`
}

// kvBindings is the KV bindings env deploys with: an environment's own, since
// wrangler does not inherit bindings into a named environment.
func (c wranglerConfig) kvBindings(env string) []kvBinding {
	if env == "" {
		return c.KV
	}
	return c.Env[env].KV
}

// config is the wrangler.toml a directory holds. Its callers all have the
// directory and none of them has the path.
func config(dir string) (wranglerConfig, error) {
	return readWrangler(filepath.Join(dir, ConfigFile))
}

func readWrangler(path string) (wranglerConfig, error) {
	cfg, err := conf.Load[wranglerConfig](path)
	if err != nil {
		return cfg, err
	}
	if cfg.Name == "" {
		return cfg, fmt.Errorf("%s has no name; add one", path)
	}
	return cfg, nil
}

// workerName is the Worker wrangler deploys env to: the environment's own
// name when it sets one, otherwise <name>-<env>, and plain <name> for the
// top-level environment; with the developer's suffix, when set.
func workerName(cfg wranglerConfig, env string) string {
	if env == "" {
		return suffix.Apply(cfg.Name)
	}
	if e, ok := cfg.Env[env]; ok && e.Name != "" {
		return suffix.Apply(e.Name)
	}
	return suffix.Apply(cfg.Name) + "-" + env
}

// Name is the Worker that dir's config deploys env to, suffix included.
func Name(dir, env string) (string, error) {
	cfg, err := config(dir)
	if err != nil {
		return "", err
	}
	return workerName(cfg, env), nil
}

// URL is the deployed address of the Worker in dir: its workers.dev name for
// this account and environment.
//
// It used to take the local address and a flag saying which of the two was
// wanted, and answered the empty local one with an empty string — so `dev url
// DIR` printed a blank line and exited 0. Choosing between local and deployed
// is the same choice on every cloud, so package app makes it once and this
// answers the only question that is Cloudflare's.
func URL(dir, env string, refresh bool) (string, error) {
	cfg, err := config(dir)
	if err != nil {
		return "", err
	}
	sub, err := subdomain(refresh)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://%s.%s.workers.dev", workerName(cfg, env), sub), nil
}

// subdomain is the account's workers.dev subdomain: the one fact a URL needs
// that no committed file may hold. It is read from the API once and kept in
// mise.local.toml.
func subdomain(refresh bool) (string, error) {
	if !refresh {
		env, err := readLocal(localFile)
		if err != nil {
			return "", err
		}
		if env[subdomainKey] != "" {
			return env[subdomainKey], nil
		}
	}
	sub, err := fetchSubdomain()
	if err != nil {
		return "", err
	}
	if err := writeLocal(localFile, subdomainKey, sub); err != nil {
		return "", err
	}
	return sub, nil
}

func fetchSubdomain() (string, error) {
	result, err := ask[struct {
		Subdomain string `json:"subdomain"`
	}]("the account's workers.dev subdomain", subdomainEndpoint)
	if err != nil {
		return "", err
	}
	if result.Subdomain == "" {
		return "", errors.New("this account has no workers.dev subdomain; Cloudflare asks for one on the Workers dashboard before a Worker has a URL")
	}
	return result.Subdomain, nil
}

// mise.local.toml is this clone's, gitignored, and mise reads its [env] like
// any other config file, so what dev learns about the account is also in the
// shell environment.

type localConfig struct {
	Env map[string]string `toml:"env"`
}

func readLocal(path string) (map[string]string, error) {
	c, err := conf.Load[localConfig](path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("%w (it is this clone's file; fix or delete it)", err)
	}
	if c.Env == nil {
		c.Env = map[string]string{}
	}
	return c.Env, nil
}

func writeLocal(path, key, value string) error {
	env, err := readLocal(path)
	if err != nil {
		return err
	}
	env[key] = value
	keys := cli.SortedKeys(env)
	var b strings.Builder
	b.WriteString("# Written by `dev url` from the Cloudflare account in fnox. Gitignored: it is\n")
	b.WriteString("# this clone's. After switching accounts: dev url DIR --deployed --refresh\n")
	b.WriteString("[env]\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %s\n", k, strconv.Quote(env[k]))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// Asking Cloudflare's own API, for the two things wrangler will not answer.
//
// wrangler is the way everything else here reaches Cloudflare, and that is
// deliberate: it holds the credential, it does the work, and it is pinned by
// the registry. But it has no command that prints the account's workers.dev
// subdomain, and none that lists the Workers on an account — `wrangler
// deployments` wants a Worker already named. Both are one GET.
package cloudflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/joeblew999/dev/internal/fnox"
)

// The credentials these calls need. They are the same two wrangler gets from
// fnox; here dev holds them itself, because dev is making the request.
// scriptsEndpoint lists an account's Workers.
var scriptsEndpoint = "https://api.cloudflare.com/client/v4/accounts/%s/workers/scripts"

const (
	TokenEnv   = "CLOUDFLARE_API_TOKEN"
	AccountEnv = "CLOUDFLARE_ACCOUNT_ID"
)

// apiReply is the envelope Cloudflare puts around every answer. Both callers
// had written their own copy of it and their own walk over Errors.
type apiReply[T any] struct {
	Success bool `json:"success"`
	Result  T    `json:"result"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// why is what went wrong, in Cloudflare's own words when it gave any.
func (r apiReply[T]) why() string {
	msgs := make([]string, 0, len(r.Errors))
	for _, e := range r.Errors {
		msgs = append(msgs, e.Message)
	}
	if len(msgs) == 0 {
		return ""
	}
	return ": " + strings.Join(msgs, "; ")
}

// ask makes one GET against the account's API and hands back what Result held.
func ask[T any](what, endpoint string) (T, error) {
	return call[T](http.MethodGet, what, endpoint, nil)
}

// post makes one POST with a JSON body, for the endpoints that take a query
// rather than a path.
func post[T any](what, endpoint string, body any) (T, error) {
	return call[T](http.MethodPost, what, endpoint, body)
}

// call is one request against the account's API.
//
// Generic because the envelope is the same every time and only the result
// differs, which is the whole of what every caller would otherwise duplicate:
// the bearer header, the status check, the success flag, and walking the
// errors to say why.
func call[T any](method, what, endpoint string, body any) (T, error) {
	var zero T
	token, err := credential(TokenEnv)
	if err != nil {
		return zero, err
	}
	account, err := credential(AccountEnv)
	if err != nil {
		return zero, err
	}
	var send io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return zero, err
		}
		send = bytes.NewReader(encoded)
	}
	return sent[T](method, what, fmt.Sprintf(endpoint, account), token, send)
}

// request is one GET against a URL that is already complete, for the endpoints
// scoped to something other than the account — a zone, most of them.
func request[T any](what, url, token string) (T, error) {
	return sent[T](http.MethodGet, what, url, token, nil)
}

// sent is the request itself: the bearer header, the status check, the
// success flag, and walking the errors to say why. Everything that reaches
// Cloudflare's API here goes through it.
func sent[T any](method, what, url, token string, send io.Reader) (T, error) {
	var zero T
	req, err := http.NewRequest(method, url, send)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if send != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("asking Cloudflare for %s: %w", what, err)
	}
	defer resp.Body.Close()
	var reply apiReply[T]
	_ = json.NewDecoder(resp.Body).Decode(&reply)
	if resp.StatusCode != http.StatusOK || !reply.Success {
		return zero, fmt.Errorf("could not read %s (HTTP %d%s); check that %s in fnox can read Workers and %s is the account that owns them",
			what, resp.StatusCode, reply.why(), TokenEnv, AccountEnv)
	}
	return reply.Result, nil
}

// credential is one of the two, out of fnox — asked once per run.
//
// fnox.Get spawns a process, which costs about 120ms, and every request here
// needs both the token and the account. Asked per request that was eighteen
// spawns to read one account's domains and most of the four seconds it took.
// A credential does not change while a command runs, so it is read when
// something first wants it and kept.
//
// Memoised per name rather than once for both, because a command that only
// ever needs the token should not be made to fetch the account as well.
// Typed, so reading it back is not an assertion that can panic. A sync.Map
// of any holds whatever anybody put in it, and the one place that reads this
// asserted the type without checking — correct today because one function
// writes it, and a panic in a CLI the first time that stops being true.
var credentials sync.Map // name -> *credential_

type credential_ struct {
	once  sync.Once
	value string
	err   error
}

// forgetCredentials drops what was read, so a test that replaces fnox gets
// the replacement rather than what a previous test left behind.
//
// Needed because the memo is exactly as invisible to a stub as it is fast: a
// test here stubbed fnox with an empty store, expected "not in fnox", and
// instead got the real token and made a live API call — which passed for
// nobody and reached the real account.
func forgetCredentials() { credentials.Clear() }

func credential(name string) (string, error) {
	c, _ := credentials.LoadOrStore(name, &credential_{})
	got, ok := c.(*credential_)
	if !ok {
		return "", fmt.Errorf("the memo for %s holds a %T; this is a bug in dev, not in your config", name, c)
	}
	got.once.Do(func() {
		v, err := fnox.Get(name)
		if err != nil || v == "" {
			got.err = fmt.Errorf("%s is not in fnox; store it with: fnox set -g %s", name, name)
			return
		}
		got.value = v
	})
	return got.value, got.err
}

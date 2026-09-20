// Asking Cloudflare's own API, for the two things wrangler will not answer.
//
// wrangler is the way everything else here reaches Cloudflare, and that is
// deliberate: it holds the credential, it does the work, and it is pinned by
// the registry. But it has no command that prints the account's workers.dev
// subdomain, and none that lists the Workers on an account — `wrangler
// deployments` wants a Worker already named. Both are one GET.
package cloudflare

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
//
// Generic because the envelope is the same every time and only the result
// differs, which is the whole of what the two callers had duplicated: the
// bearer header, the status check, the success flag, and walking the errors
// to say why.
func ask[T any](what, endpoint string) (T, error) {
	var zero T
	token, err := credential(TokenEnv)
	if err != nil {
		return zero, err
	}
	account, err := credential(AccountEnv)
	if err != nil {
		return zero, err
	}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(endpoint, account), nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("asking Cloudflare for %s: %w", what, err)
	}
	defer resp.Body.Close()
	var body apiReply[T]
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != http.StatusOK || !body.Success {
		return zero, fmt.Errorf("could not read %s (HTTP %d%s); check that %s in fnox can read Workers and %s is the account that owns them",
			what, resp.StatusCode, body.why(), TokenEnv, AccountEnv)
	}
	return body.Result, nil
}

// credential is one of the two, out of fnox.
func credential(name string) (string, error) {
	v, err := fnox.Get(name)
	if err != nil || v == "" {
		return "", fmt.Errorf("%s is not in fnox; store it with: fnox set -g %s", name, name)
	}
	return v, nil
}

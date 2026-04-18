# JWKS Stub for E2E Testing

In-process HTTP stub that stands in for a JWKS-based JWT auth provider during
end-to-end tests.

---

## When to Use

Services that sit behind a JWT auth provider validate bearer tokens against
public keys fetched from the provider's JWKS endpoint. In e2e tests you don't
want to depend on a live provider being reachable, and you need full control
over the claims in each token. An in-process stub solves both: it starts with
the test binary, occupies a random loopback port, and disappears when the test
process exits.

---

## Mechanism

1. At test startup, generate an RSA keypair (RS256). The private key stays
   inside the stub; the public key is served as a JWKS document.
2. Start an `httptest.Server` (or equivalent) on a random loopback port.
3. Serve `GET /.well-known/jwks.json` (or whatever path the target service
   is configured to fetch) with the public key marshalled as a JWK Set.
4. Optionally stub any companion endpoints the provider exposes (e.g. an
   API-key-to-identity resolver). Return well-formed responses for keys the
   test controls; return 404/401 for everything else — never accept arbitrary
   unknown input silently.
5. Return a `signJWT` closure. Tests call it to mint a signed RS256 token
   containing whatever claims the test needs. The service validates that token
   against the stub's public JWKS, so the signature round-trip is real.

---

## Wire-Up

The service binary should already accept a flag or environment variable that
controls which auth provider URL it contacts (e.g. `--auth-provider-url` /
`AUTH_PROVIDER_URL`). The test harness:

1. Starts the stub and records its base URL (e.g. `http://127.0.0.1:PORT`).
2. Launches the service under test with that URL substituted in.
3. Uses the `signJWT` closure to produce tokens for each test case.

No production code changes are required as long as the service is already
parameterised on the provider URL. If it isn't, that is a prerequisite change
— hard-coding auth provider URLs is an anti-pattern regardless of testing.

---

## Example Skeleton

```go
// Package authjwks provides an in-process JWKS stub for e2e tests.
package authjwks

import (
    "crypto/rand"
    "crypto/rsa"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/lestrrat-go/jwx/v2/jwa"
    "github.com/lestrrat-go/jwx/v2/jwk"
    "github.com/lestrrat-go/jwx/v2/jwt"
)

// Stub is a running JWKS stub server.
type Stub struct {
    // URL is the base URL of the stub (e.g. "http://127.0.0.1:PORT").
    URL        string
    privateKey *rsa.PrivateKey
    server     *httptest.Server
}

// Start generates an RSA keypair, starts an httptest.Server, and registers
// a cleanup function that closes the server when t finishes.
func Start(t *testing.T) *Stub {
    t.Helper()

    priv, err := rsa.GenerateKey(rand.Reader, 2048)
    if err != nil {
        t.Fatalf("authjwks: generate RSA key: %v", err)
    }

    pub, err := jwk.FromRaw(&priv.PublicKey)
    if err != nil {
        t.Fatalf("authjwks: build JWK: %v", err)
    }
    _ = pub.Set(jwk.KeyIDKey, "test-key-1")
    _ = pub.Set(jwk.AlgorithmKey, jwa.RS256)

    keySet := jwk.NewSet()
    _ = keySet.AddKey(pub)

    jwksJSON, err := json.Marshal(keySet)
    if err != nil {
        t.Fatalf("authjwks: marshal JWKS: %v", err)
    }

    mux := http.NewServeMux()
    mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write(jwksJSON)
    })

    srv := httptest.NewServer(mux)
    t.Cleanup(srv.Close)

    return &Stub{
        URL:        srv.URL,
        privateKey: priv,
        server:     srv,
    }
}

// SignJWT mints a signed RS256 JWT with the given subject, audience, and
// issuer. Add any additional claims via the returned builder before calling
// Build — or extend this signature to accept a claims map.
func (s *Stub) SignJWT(t *testing.T, subject, issuer, audience string) string {
    t.Helper()

    tok, err := jwt.NewBuilder().
        Subject(subject).
        Issuer(issuer).
        Audience([]string{audience}).
        IssuedAt(time.Now()).
        Expiration(time.Now().Add(15 * time.Minute)).
        Build()
    if err != nil {
        t.Fatalf("authjwks: build token: %v", err)
    }

    signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, s.privateKey))
    if err != nil {
        t.Fatalf("authjwks: sign token: %v", err)
    }

    return string(signed)
}
```

Usage in a test:

```go
func TestProtectedEndpoint(t *testing.T) {
    stub := authjwks.Start(t)

    svc := startService(t, "--auth-provider-url", stub.URL)

    tok := stub.SignJWT(t, "user-123", stub.URL, "my-service")
    resp, err := http.Get(svc.URL + "/api/resource",
        // add Authorization: Bearer <tok> header
    )
    // assert 200
}
```

---

## Pitfalls

- **Companion endpoints should reject unknown input.** If the provider exposes
  an API-key-to-identity resolver, stub it to return 401 for keys it doesn't
  recognise. Returning a valid identity for any input means a misconfigured
  test can silently succeed.
- **Include `iss` and `aud` claims.** Most services validate issuer and
  audience. Omitting them is the most common reason tokens are rejected with
  a cryptic 401. Pass the stub's own URL as `iss` (or whatever the service
  expects) and the service's intended audience as `aud`.
- **Negative auth tests need real rejection.** If you test that the service
  returns 401 on a bad token, send a token signed with a *different* key —
  the stub's JWKS will not contain a matching key, so validation fails
  naturally. Don't fake a 401 from the stub itself.
- **Key ID (`kid`) matching.** Some JWT libraries require the `kid` in the
  token header to match a key in the JWKS. Set the same `kid` value in
  `SignJWT` (via `jwt.WithKey(..., jwk.WithKeyID("test-key-1"))`) as was
  set on the public JWK.
- **Token expiry in slow CI.** Use a generous `exp` (e.g. 15 minutes) or
  allow the service's clock tolerance to absorb it. Avoid mocking `time.Now`
  unless the service itself supports clock injection.

---

## See Also

- `lestrrat-go/jwx` docs: <https://github.com/lestrrat-go/jwx>
- RFC 7517 (JWK), RFC 7519 (JWT), RFC 7518 (RS256)
- @references/architecture-reference.md — testing patterns section for the
  broader test harness conventions used in this stack

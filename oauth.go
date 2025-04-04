package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	cf "github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/accounts"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"golang.org/x/oauth2"
)

var (
	cfClient     *cf.Client
	cfAccount    *accounts.Account
	state        string
	codeVerifier string
	tokenChan    = make(chan *oauth2.Token)
	config       = &oauth2.Config{
		ClientID:     "54d11594-84e4-41aa-b438-e81b8fa78ee7",
		ClientSecret: "",
		RedirectURL:  "http://localhost:8976/oauth/callback",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://dash.cloudflare.com/oauth2/auth",
			TokenURL: "https://dash.cloudflare.com/oauth2/token",
		},
		Scopes: []string{
			"account:read", "user:read", "workers:write", "workers_kv:write",
			"workers_routes:write", "workers_scripts:write", "workers_tail:read",
			"d1:write", "pages:write", "zone:read", "ssl_certs:write",
			"ai:write", "queues:write", "pipelines:write", "secrets_store:write",
		},
	}
)

func NewClient(token oauth2.Token) *cf.Client {
	return cf.NewClient(option.WithAPIToken(token.AccessToken))
}

func getAccount(ctx context.Context) (*accounts.Account, error) {
	res, err := cfClient.Accounts.List(ctx, accounts.AccountListParams{})
	if err != nil {
		return nil, err
	}

	return &res.Result[0], nil
}

func generateAuthURL() string {
	state = generateState()
	codeVerifier = generateCodeVerifier()
	codeChallenge := generateCodeChallenge(codeVerifier)

	return config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

func generateState() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	buff := make([]byte, 16)
	for i := range buff {
		buff[i] = charset[rand.Int64()%int64(len(charset))]
	}

	return base64.URLEncoding.EncodeToString(buff)
}

func generateCodeVerifier() string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(rand.IntN(256))
	}

	return base64.RawURLEncoding.EncodeToString(b)
}

func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func login(ctx context.Context) error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	url := generateAuthURL()
	fmt.Printf("\n%s Login %sCloudflare%s...\n", title, orange, reset)
	if err := openURL(url); err != nil {
		return err
	}

	return nil
}

func callback(w http.ResponseWriter, r *http.Request) {
	var err error
	param := r.URL.Query().Get("state")
	if param != state {
		failMessage("Invalid OAuth state", nil)

		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		failMessage("No code returned", nil)

		return
	}

	token, err := config.Exchange(
		context.Background(),
		code,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)

	if err != nil {
		failMessage("Error exchanging oauthToken", err)
	}

	tokenChan <- token
	successMessage("Cloudflare logged in successfully!")
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<script>window.close();</script>`)
}

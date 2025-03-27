package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	cf "github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/accounts"
	"github.com/cloudflare/cloudflare-go/v4/kv"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"github.com/cloudflare/cloudflare-go/v4/pages"
	"github.com/cloudflare/cloudflare-go/v4/workers"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

var (
	red    = "\033[31m"
	green  = "\033[32m"
	reset  = "\033[0m"
	orange = "\033[38;2;255;165;0m"
	blue   = "\033[94m"
	bold   = "\033[1m"
	title  = bold + blue + "●" + reset
	ask    = bold + "-" + reset
	info   = bold + "+" + reset
)

var (
	cfClient       *cf.Client
	cfAccount      *accounts.Account
	codeVerifier   string
	oauthState     string
	oauthTokenChan = make(chan *oauth2.Token)
	oauthConfig    = &oauth2.Config{
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

type ProjectDeploymentNewParams struct {
	AccountID string                `form:"account_id,required"`
	Branch    string                `form:"branch"`
	Manifest  string                `form:"manifest"`
	WorkerJS  *multipart.FileHeader `form:"_worker.js"`
	Path      string
}

func (pdp ProjectDeploymentNewParams) MarshalMultipart() ([]byte, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	manifestHeaders := textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="manifest"`},
	}
	manifestPart, err := writer.CreatePart(manifestHeaders)
	if err != nil {
		return nil, "", fmt.Errorf("error creating manifest part: %w", err)
	}
	_, err = manifestPart.Write([]byte("{}"))
	if err != nil {
		return nil, "", fmt.Errorf("error writing manifest content: %w", err)
	}
	branchHeaders := textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="branch"`},
	}
	branchPart, err := writer.CreatePart(branchHeaders)
	if err != nil {
		return nil, "", fmt.Errorf("error creating branch part: %w", err)
	}
	_, err = branchPart.Write([]byte("main"))
	if err != nil {
		return nil, "", fmt.Errorf("error writing branch content: %w", err)
	}
	fileHeaders := textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="_worker.js"; filename="_worker.js"`},
		"Content-Type":        []string{"application/javascript"},
	}
	filePart, err := writer.CreatePart(fileHeaders)
	if err != nil {
		return nil, "", fmt.Errorf("error creating file part: %w", err)
	}
	file, err := os.Open(pdp.Path)
	if err != nil {
		return nil, "", fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()
	_, err = io.Copy(filePart, file)
	if err != nil {
		return nil, "", fmt.Errorf("error copying file content: %w", err)
	}
	err = writer.Close()
	if err != nil {
		return nil, "", fmt.Errorf("error closing multipart writer: %w", err)
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func callback(w http.ResponseWriter, r *http.Request) {
	var err error
	state := r.URL.Query().Get("state")
	if state != oauthState {
		failMessage("Invalid OAuth state", nil)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		failMessage("No code returned", nil)
		return
	}
	token, err := oauthConfig.Exchange(
		context.Background(),
		code,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		failMessage("Error exchanging oauthToken", err)
	}
	oauthTokenChan <- token
	w.Write([]byte("Obtained token successfully."))
}

func deployBPB(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	select {
	case <-ctx.Done():
		return
	case token := <-oauthTokenChan:
		cfClient = cf.NewClient(option.WithAPIToken(token.AccessToken))
		res, err := cfClient.Accounts.List(ctx, accounts.AccountListParams{})
		if err != nil {
			failMessage("Error getting oauthToken", err)
			return
		}
		if len(res.Result) != 0 {
			cfAccount = &res.Result[0]
		}
		installDir, err := os.MkdirTemp("", "bpb-wizard")
		if err != nil {
			failMessage("Error creating temp directory", err)
			return
		}
		workerURL := "https://github.com/bia-pain-bache/BPB-Worker-Panel/releases/latest/download/worker.js"
		workerPath := filepath.Join(installDir, "worker.js")
		fmt.Printf("\n%s Downloading %sworker.js%s...\n", title, green, reset)
		for {
			if err := downloadFile(workerURL, workerPath); err != nil {
				failMessage("Error downloading worker.js", err)
				if response := promptUser("Would you like to try again? (y/n): "); strings.ToLower(response) == "n" {
					return
				}
				continue
			}
			successMessage("Worker downloaded successfully!")
			break
		}
		fmt.Printf("\n%s Get Worker settings...\n", title)
		var workerName string
		for {
			workerName = generateRandomDomain(32)
			fmt.Printf("\n%s The random generated worker name (%sSubdomain%s) is: %s%s%s\n", info, green, reset, orange, workerName, reset)
			if response := promptUser("Please enter a custom worker name or press ENTER to use generated one: "); response != "" {
				if strings.Contains(strings.ToLower(response), "bpb") {
					message := fmt.Sprintf("Worker name cannot contain %sbpb%s! Please try another name.", red, reset)
					failMessage(message, nil)
					continue
				}
				workerName = response
			}
			if resp := isWorkerAvailable(ctx, workerName); resp {
				prompt := fmt.Sprintf("This worker already exists! This will %sRESET%s all panel settings, would you like to override it? (y/n): ", red, reset)
				if response := promptUser(prompt); strings.ToLower(response) == "n" {
					continue
				}
			}
			break
		}

		uid := uuid.NewString()
		fmt.Printf("\n%s The random generated %sUUID%s is: %s%s%s\n", info, green, reset, orange, uid, reset)
		if response := promptUser("Please enter a custom UUID or press ENTER to use generated one: "); response != "" {
			uid = response
		}

		pass := generateTrPassword(12)
		fmt.Printf("\n%s The random generated %sTrojan password%s is: %s%s%s\n", info, green, reset, orange, pass, reset)
		if response := promptUser("Please enter a custom Trojan password or press ENTER to use generated one: "); response != "" {
			pass = response
		}

		proxyIP := "bpb.yousef.isegaro.com"
		fmt.Printf("\n%s The default %sProxy IP%s is: %s%s%s\n", info, green, reset, orange, proxyIP, reset)
		if response := promptUser("Please enter custom Proxy IP/Domains or press ENTER to use default: "); response != "" {
			proxyIP = response
		}

		fallback := "speed.cf.com"
		fmt.Printf("\n%s The default %sFallback domain%s is: %s%s%s\n", info, green, reset, orange, fallback, reset)
		if response := promptUser("Please enter a custom Fallback domain or press ENTER to use default: "); response != "" {
			fallback = response
		}

		subPath := generateSubURIPath(16)
		fmt.Printf("\n%s The random generated %sSubscription path%s is: %s%s%s\n", info, green, reset, orange, subPath, reset)
		if response := promptUser("Please enter a custom Subscription path or press ENTER to use generated one: "); response != "" {
			subPath = response
		}

		fmt.Printf("\n%s Creating KV namespace...\n", title)
		var kvNameSpace *kv.Namespace
		for {
			now := time.Now().Format("2006-01-02_15-04-05")
			kvName := fmt.Sprintf("panel-kv-%s", now)
			kvNameSpace, err = createKVNamespace(ctx, kvName)
			if err != nil {
				failMessage("Error creating KV!", err)
				if response := promptUser("Would you like to try again? (y/n): "); strings.ToLower(response) == "n" {
					return
				}
				continue
			}
			break
		}
		successMessage("KV created successfully!")
		var project *pages.Project
		for {
			fmt.Printf("\n%s Deploying Page...\n", title)
			project, err = createPage(ctx, workerName, uid, pass, proxyIP, fallback, subPath, kvNameSpace)
			if err != nil {
				failMessage("Error deploying page", err)
				if response := promptUser("Would you like to try again? (y/n): "); strings.ToLower(response) == "n" {
					return
				}
				continue
			}
			successMessage("Page created successfully!")
			break
		}

		for {
			_, err = deployPage(ctx, project, workerPath)
			if err != nil {
				failMessage("Error deploying page", err)
				if response := promptUser("Would you like to try again? (y/n): "); strings.ToLower(response) == "n" {
					return
				}
				continue
			}
			successMessage("Page deployed successfully!")
			break
		}
		successMessage("All Done, Now we wait")
	}
}

func promptUser(prompt string) string {
	fmt.Printf("%s %s", ask, prompt)
	var response string
	fmt.Scanln(&response)
	return strings.TrimSpace(response)
}

func generateRandomString(charSet string, length int, isDomain bool) string {
	randomBytes := make([]byte, length)
	for i := range randomBytes {
		for {
			char := charSet[rand.IntN(len(charSet))]
			if isDomain && (i == 0 || i == length-1) && char == byte('-') {
				continue
			}
			randomBytes[i] = char
			break
		}
	}

	return string(randomBytes)
}

func generateAuthURL() string {
	oauthState = generateState()
	codeVerifier = generateCodeVerifier()
	codeChallenge := generateCodeChallenge(codeVerifier)
	return oauthConfig.AuthCodeURL(
		oauthState,
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

func generateRandomDomain(subDomainLength int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789-"
	return generateRandomString(charset, subDomainLength, true)
}

func generateTrPassword(passwordLength int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()_+[]{}|;:',.<>?"
	return generateRandomString(charset, passwordLength, false)
}

func generateSubURIPath(uriLength int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@$&*_-+;:,."
	return generateRandomString(charset, uriLength, false)
}

func isWorkerAvailable(ctx context.Context, workerName string) bool {
	_, err := cfClient.Workers.Scripts.Deployments.Get(ctx, workerName, workers.ScriptDeploymentGetParams{AccountID: cf.F(cfAccount.ID)})
	if err == nil {
		return true
	}
	return false
}

func createKVNamespace(ctx context.Context, ns string) (*kv.Namespace, error) {
	res, err := cfClient.KV.Namespaces.New(ctx, kv.NamespaceNewParams{AccountID: cf.F(cfAccount.ID), Title: cf.F(ns)})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func createPage(ctx context.Context, name string, uid string, pass string, proxy string, fallback string, sub string, kv *kv.Namespace) (*pages.Project, error) {
	return cfClient.Pages.Projects.New(
		ctx,
		pages.ProjectNewParams{
			AccountID: cf.F(cfAccount.ID),
			Project: pages.ProjectParam{
				Name:             cf.F(name),
				ProductionBranch: cf.F("main"),
				DeploymentConfigs: cf.F(pages.ProjectDeploymentConfigsParam{
					Production: cf.F(pages.ProjectDeploymentConfigsProductionParam{
						CompatibilityDate:  cf.F(time.Now().AddDate(0, 0, -1).Format("2006-01-02")),
						CompatibilityFlags: cf.F([]string{"nodejs_compat"}),
						KVNamespaces: cf.F(map[string]pages.ProjectDeploymentConfigsProductionKVNamespaceParam{
							"kv": {
								NamespaceID: cf.F(kv.ID),
							},
						}),
						Services: cf.F(map[string]pages.ProjectDeploymentConfigsProductionServiceParam{}),
						EnvVars: cf.F(map[string]pages.ProjectDeploymentConfigsProductionEnvVarsUnionParam{
							"UUID": pages.ProjectDeploymentConfigsProductionEnvVarsPagesPlainTextEnvVarParam{
								Type:  cf.F(pages.ProjectDeploymentConfigsProductionEnvVarsPagesPlainTextEnvVarTypePlainText),
								Value: cf.F(uid),
							},
							"TR_PASS": pages.ProjectDeploymentConfigsProductionEnvVarsPagesPlainTextEnvVarParam{
								Type:  cf.F(pages.ProjectDeploymentConfigsProductionEnvVarsPagesPlainTextEnvVarTypePlainText),
								Value: cf.F(pass),
							},
							"PROXY_IP": pages.ProjectDeploymentConfigsProductionEnvVarsPagesPlainTextEnvVarParam{
								Type:  cf.F(pages.ProjectDeploymentConfigsProductionEnvVarsPagesPlainTextEnvVarTypePlainText),
								Value: cf.F(proxy),
							},
							"FALLBACK": pages.ProjectDeploymentConfigsProductionEnvVarsPagesPlainTextEnvVarParam{
								Type:  cf.F(pages.ProjectDeploymentConfigsProductionEnvVarsPagesPlainTextEnvVarTypePlainText),
								Value: cf.F(fallback),
							},
							"SUB_PATH": pages.ProjectDeploymentConfigsProductionEnvVarsPagesPlainTextEnvVarParam{
								Type:  cf.F(pages.ProjectDeploymentConfigsProductionEnvVarsPagesPlainTextEnvVarTypePlainText),
								Value: cf.F(sub),
							},
						}),
					}),
				}),
			},
		})
}

func deployPage(ctx context.Context, project *pages.Project, assetsPath string) (*pages.Deployment, error) {
	param := ProjectDeploymentNewParams{AccountID: cfAccount.ID, Branch: "main", Manifest: "{}", WorkerJS: &multipart.FileHeader{Filename: "worker.js"}, Path: assetsPath}
	data, ct, err := param.MarshalMultipart()
	if err != nil {
		return nil, err
	}
	r := bytes.NewBuffer(data)
	return cfClient.Pages.Projects.Deployments.New(
		ctx,
		project.Name,
		pages.ProjectDeploymentNewParams{AccountID: cf.F(cfAccount.ID)},
		option.WithRequestBody(ct, r),
	)
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error making GET request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: %s (HTTP %d)", url, resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer out.Close()

	if _, err = io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("error writing to file: %v", err)
	}

	return nil
}

func openURL(url string) error {
	var cmd string
	var args = []string{url}
	switch runtime.GOOS {
	case "darwin": // MacOS
		cmd = "open"
		args = []string{url}
	case "windows": // Windows
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default: // Linux, BSD, etc.
		cmd = "xdg-open"
		args = []string{url}
	}
	return exec.Command(cmd, args...).Start()
}

func failMessage(message string, err error) {
	errMark := bold + red + "✗" + reset
	if err != nil {
		fmt.Printf("%s %s: %s\n", errMark, message, err)
	}

	fmt.Printf("%s %s\n", errMark, message)
}

func successMessage(message string) {
	succMark := bold + green + "✓" + reset
	fmt.Printf("%s %s\n", succMark, message)
}

func main() {
	ctx := context.Background()
	go func() {
		url := generateAuthURL()
		if err := openURL(url); err != nil {
			failMessage("Error opening URL", err)
		}
	}()
	go deployBPB(ctx)
	http.HandleFunc("/oauth/callback", callback)
	err := http.ListenAndServe(":8976", nil)
	if err != nil {
		failMessage("Error starting server", err)
	}
}

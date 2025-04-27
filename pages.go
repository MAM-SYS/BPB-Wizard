package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	cf "github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/kv"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"github.com/cloudflare/cloudflare-go/v4/pages"
	"github.com/joeguo/tldextract"
)

type projectDeploymentNewParams struct {
	AccountID string                `form:"account_id,required"`
	Branch    string                `form:"branch"`
	Manifest  string                `form:"manifest"`
	WorkerJS  *multipart.FileHeader `form:"_worker.js"`
	jsPath    string
}

func (pdp projectDeploymentNewParams) MarshalMultipart() ([]byte, string, error) {
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

	file, err := os.Open(pdp.jsPath)
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

func createPageDeployment(ctx context.Context, project *pages.Project, jsPath string) (*pages.Deployment, error) {
	param := projectDeploymentNewParams{AccountID: cfAccount.ID, Branch: "main", Manifest: "{}", WorkerJS: &multipart.FileHeader{Filename: "worker.js"}, jsPath: jsPath}
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

func addPagesCustomDomain(ctx context.Context, projectName string, customDomain string, srcPath string) (string, error) {
	cacheFile := filepath.Join(srcPath, "tld.cache")
	extractor, err := tldextract.New(cacheFile, false)
	if err != nil {
		panic(err)
	}

	result := extractor.Extract(customDomain)
	// domain := fmt.Sprintf("%s.%s", result.Root, result.Tld)

	// zones, err := cfClient.Zones.List(ctx, zones.ZoneListParams{
	// 	Account: cf.F(zones.ZoneListParamsAccount{
	// 		ID: cf.F(cfAccount.ID),
	// 	}),
	// 	Match: cf.F(zones.ZoneListParamsMatch("contains")),
	// 	Name:  cf.F(domain),
	// })

	// if err != nil {
	// 	return "", err
	// }

	// if len(zones.Result) == 0 {
	// 	message := fmt.Sprintf("Could not find this domain in your account: %s", domain)
	// 	return "", fmt.Errorf(message, nil)
	// }

	// zone := zones.Result[0]
	// pagesHost := fmt.Sprintf("%s.pages.dev", projectName)

	// _, er := cfClient.DNS.Records.New(ctx, dns.RecordNewParams{
	// 	ZoneID: cf.F(zone.ID),
	// 	Record: dns.CNAMERecordParam{
	// 		Content: cf.F(pagesHost),
	// 		Name:    cf.F(customDomain),
	// 		Proxied: cf.F(true),
	// 		Type:    cf.F(dns.CNAMERecordType("CNAME")),
	// 	},
	// }, cfClient.Options...)

	// if er != nil {
	// 	return "", er
	// }

	_, e := cfClient.Pages.Projects.Domains.New(ctx, projectName, pages.ProjectDomainNewParams{
		AccountID: cf.F(cfAccount.ID),
		Name:      cf.F(customDomain),
	})

	if e != nil {
		return "", e
	}

	return result.Sub, nil
}

func isPageAvailable(ctx context.Context, projectName string) bool {
	if _, err := cfClient.Pages.Projects.Get(ctx, projectName, pages.ProjectGetParams{AccountID: cf.F(cfAccount.ID)}); err != nil {
		return true
	}

	return false
}

func deployBPBPage(ctx context.Context, name string, uid string, pass string, proxy string, fallback string, sub string, jsPath string, kvNamespace *kv.Namespace, customDomain string, srcPath string) string {
	var project *pages.Project
	var err error

	for {
		fmt.Printf("\n%s Creating Page...\n", title)

		project, err = createPage(ctx, name, uid, pass, proxy, fallback, sub, kvNamespace)
		if err != nil {
			failMessage("Error deploying page", err)
			if response := promptUser("Would you like to try again? (y/n): "); strings.ToLower(response) == "n" {
				return ""
			}
			continue
		}

		successMessage("Page created successfully!")
		break
	}

	for {
		fmt.Printf("\n%s Deploying Page...\n", title)

		_, err = createPageDeployment(ctx, project, jsPath)
		if err != nil {
			failMessage("Error deploying page", err)
			if response := promptUser("Would you like to try again? (y/n): "); strings.ToLower(response) == "n" {
				return ""
			}
			continue
		}

		successMessage("Page deployed successfully!")
		break
	}

	if customDomain != "" {
		for {
			recordName, err := addPagesCustomDomain(ctx, name, customDomain, srcPath)
			if err != nil {
				failMessage("Error adding custom domain.", err)
				if response := promptUser("Would you like to try again? (y/n): "); strings.ToLower(response) == "n" {
					return ""
				}
				continue
			}

			successMessage("Custom domain added to pages successfully!")
			// recordName := strings.Split(customDomain, ".")[0]
			fmt.Printf("%s %sWarning%s: You should create a CNAME record with Name: %s%s%s and Target: %s%s.pages.dev%s, Otherwise your Custom Domain will not work.\n", info, red, reset, green, recordName, reset, green, name, reset)
			return "https://" + customDomain + "/panel"
		}
	}

	successMessage("It takes up to 5 minutes to open panel, please wait...")
	return "https://" + project.Subdomain + "/panel"
}

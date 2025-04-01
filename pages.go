package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"time"

	cf "github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/kv"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"github.com/cloudflare/cloudflare-go/v4/pages"
)

type projectDeploymentNewParams struct {
	AccountID string                `form:"account_id,required"`
	Branch    string                `form:"branch"`
	Manifest  string                `form:"manifest"`
	WorkerJS  *multipart.FileHeader `form:"_worker.js"`
	Path      string
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

func createPageDeployment(ctx context.Context, project *pages.Project, assetsPath string) (*pages.Deployment, error) {
	param := projectDeploymentNewParams{AccountID: cfAccount.ID, Branch: "main", Manifest: "{}", WorkerJS: &multipart.FileHeader{Filename: "worker.js"}, Path: assetsPath}
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

func isPageAvailable(ctx context.Context, projectName string) bool {
	if _, err := cfClient.Pages.Projects.Get(ctx, projectName, pages.ProjectGetParams{AccountID: cf.F(cfAccount.ID)}); err != nil {
		return true
	}

	return false
}

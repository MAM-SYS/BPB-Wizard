package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"strings"
	"time"

	cf "github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/kv"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"github.com/cloudflare/cloudflare-go/v4/workers"
)

type ScriptUpdateParams struct {
	AccountID string                         `form:"account_id,required"`
	Metadata  ScriptUpdateParamsMetadataForm `form:"metadata,required"`
}

type ScriptUpdateParamsMetadataForm struct {
	MainModule         string              `form:"main_module"`
	Bindings           []map[string]string `form:"bindings"`
	CompatibilityDate  string              `form:"compatibility_date"`
	CompatibilityFlags []string            `form:"compatibility_flags"`
	Observability      map[string]bool     `form:"observability"`
	Placement          map[string]string   `form:"placement"`
	UsageModel         string              `form:"usage_model"`
	Tags               []string            `form:"tags"`
	TailConsumers      []string            `form:"tail_consumers"`
	Logpush            bool                `form:"logpush"`
	jsPath             string
}

func (sp ScriptUpdateParams) MarshalMultipart() ([]byte, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	metadata := map[string]interface{}{
		"main_module":         sp.Metadata.MainModule,
		"bindings":            sp.Metadata.Bindings,
		"compatibility_date":  sp.Metadata.CompatibilityDate,
		"compatibility_flags": sp.Metadata.CompatibilityFlags,
		"observability":       sp.Metadata.Observability,
		"placement":           sp.Metadata.Placement,
		"usage_model":         sp.Metadata.UsageModel,
		"tags":                sp.Metadata.Tags,
		"tail_consumers":      sp.Metadata.TailConsumers,
		"logpush":             sp.Metadata.Logpush,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, "", fmt.Errorf("error marshalling metadata: %w", err)
	}

	metadataHeaders := textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="metadata"`},
	}
	metadataPart, err := writer.CreatePart(metadataHeaders)
	if err != nil {
		return nil, "", fmt.Errorf("error creating metadata part: %w", err)
	}

	_, err = metadataPart.Write(metadataJSON)
	if err != nil {
		return nil, "", fmt.Errorf("error writing metadata content: %w", err)
	}

	packageLockHeaders := textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="package-lock.json"; filename="package-lock.json"`},
		"Content-Type":        []string{"text/plain"},
	}
	packageLockPart, err := writer.CreatePart(packageLockHeaders)
	if err != nil {
		return nil, "", fmt.Errorf("error creating package-lock.json part: %w", err)
	}

	_, err = packageLockPart.Write([]byte("content of package-lock.json"))
	if err != nil {
		return nil, "", fmt.Errorf("error writing package-lock.json content: %w", err)
	}

	packageJSONHeaders := textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="package.json"; filename="package.json"`},
		"Content-Type":        []string{"text/plain"},
	}
	packageJSONPart, err := writer.CreatePart(packageJSONHeaders)
	if err != nil {
		return nil, "", fmt.Errorf("error creating package.json part: %w", err)
	}

	_, err = packageJSONPart.Write([]byte("content of package.json"))
	if err != nil {
		return nil, "", fmt.Errorf("error writing package.json content: %w", err)
	}

	fileHeaders := textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="worker.js"; filename="worker.js"`},
		"Content-Type":        []string{"application/javascript+module"},
	}
	filePart, err := writer.CreatePart(fileHeaders)
	if err != nil {
		return nil, "", fmt.Errorf("error creating file part: %w", err)
	}

	file, err := os.Open(sp.Metadata.jsPath)
	if err != nil {
		return nil, "", fmt.Errorf("error opening file: %w", err)
	}

	defer file.Close()
	_, err = io.Copy(filePart, file)
	if err != nil {
		return nil, "", fmt.Errorf("error copying file content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("error closing multipart writer: %w", err)
	}

	return body.Bytes(), writer.FormDataContentType(), nil
}

func createWorker(ctx context.Context, name string, uid string, pass string, proxy string, fallback string, sub string, jsPath string, kv *kv.Namespace) (*workers.ScriptUpdateResponse, error) {
	param := ScriptUpdateParams{
		AccountID: cfAccount.ID,
		Metadata: ScriptUpdateParamsMetadataForm{
			Bindings: []map[string]string{
				{
					"name":         "kv",
					"namespace_id": kv.ID,
					"type":         "kv_namespace",
				},
				{
					"name": "UUID",
					"text": uid,
					"type": "plain_text",
				},
				{
					"name": "TR_PASS",
					"text": pass,
					"type": "plain_text",
				},
				{
					"name": "PROXY_IP",
					"text": proxy,
					"type": "plain_text",
				},
				{
					"name": "FALLBACK",
					"text": fallback,
					"type": "plain_text",
				},
				{
					"name": "SUB_PATH",
					"text": sub,
					"type": "plain_text",
				},
			},
			MainModule:        "worker.js",
			jsPath:            jsPath,
			CompatibilityDate: time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
			CompatibilityFlags: []string{
				"nodejs_compat",
			},
			Observability: map[string]bool{"enabled": false},
			Placement:     map[string]string{},
			Tags:          []string{},
			TailConsumers: []string{},
			Logpush:       false,
			UsageModel:    "standard",
		},
	}

	data, ct, err := param.MarshalMultipart()
	r := bytes.NewBuffer(data)
	if err != nil {
		return nil, err
	}

	result, err := cfClient.Workers.Scripts.Update(
		ctx,
		name,
		workers.ScriptUpdateParams{AccountID: cf.F(cfAccount.ID)},
		option.WithRequestBody(ct, r),
		option.WithHeader("Content-Type", ct),
	)
	if err != nil {
		return nil, err
	}

	return result, nil

}

func createKVNamespace(ctx context.Context, ns string) (*kv.Namespace, error) {
	res, err := cfClient.KV.Namespaces.New(ctx, kv.NamespaceNewParams{AccountID: cf.F(cfAccount.ID), Title: cf.F(ns)})
	if err != nil {
		return nil, err
	}

	return res, nil
}

func enableWorkerSubdomain(ctx context.Context, name string) (*workers.ScriptSubdomainNewResponse, error) {
	return cfClient.Workers.Scripts.Subdomain.New(
		ctx,
		name,
		workers.ScriptSubdomainNewParams{
			AccountID:       cf.F(cfAccount.ID),
			Enabled:         cf.F(true),
			PreviewsEnabled: cf.F(false),
		})
}

// func updateWorkerSubDomain(ctx context.Context, domain string) (string, error) {
// 	res, err := cfClient.Workers.Subdomains.Update(ctx, workers.SubdomainUpdateParams{AccountID: cf.F(cfAccount.ID), Subdomain: cf.F(domain)})
// 	if err != nil {
// 		return "", err
// 	}

// 	return res.Subdomain, nil
// }

func isWorkerAvailable(ctx context.Context, name string) bool {
	_, err := cfClient.Workers.Scripts.Get(ctx, name, workers.ScriptGetParams{AccountID: cf.F(cfAccount.ID)})
	return err != nil
}

func deployBPBWorker(ctx context.Context, name string, uid string, pass string, proxy string, fallback string, sub string, jsPath string, kvNamespace *kv.Namespace) string {
	for {
		fmt.Printf("\n%s Creating Worker...\n", title)
		_, err := createWorker(ctx, name, uid, pass, proxy, fallback, sub, jsPath, kvNamespace)
		if err != nil {
			failMessage("Error deploying worker", err)
			if response := promptUser("Would you like to try again? (y/n): "); strings.ToLower(response) == "n" {
				return ""
			}
			continue
		}
		successMessage("Worker created successfully!")
		break
	}

	for {
		_, err := enableWorkerSubdomain(ctx, name)
		if err != nil {
			failMessage("Error enabling worker subdomain", err)
			if response := promptUser("Would you like to try again? (y/n): "); strings.ToLower(response) == "n" {
				return ""
			}
			continue
		}
		successMessage("Worker subdomain enabled successfully!")
		break
	}

	// for {
	// 	var err error
	// 	_, err = updateWorkerSubDomain(ctx, domain)
	// 	if err != nil {
	// 		failMessage("Error updating worker subdomain", err)
	// 		if response := promptUser("Would you like to try again? (y/n): "); strings.ToLower(response) == "n" {
	// 			return ""
	// 		}
	// 		continue
	// 	}
	// 	successMessage("Worker subdomain customized successfully!")
	// 	break
	// }

	resp, err := cfClient.Workers.Subdomains.Get(ctx, workers.SubdomainGetParams{AccountID: cf.F(cfAccount.ID)})
	if err != nil {
		failMessage("Error getting worker subdomain", err)

		return ""
	}

	return "https://" + name + "." + resp.Subdomain + ".workers.dev/panel"
}

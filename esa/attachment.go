package esa

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

const (
	// ファイルアップロードのポリシーを問い合わせるURL
	AttchmentPolicyURL = "/attachments/policies"
	// ファイルアップロードのポリシーを取得する際のデータタイプ
	PolicyBodyType = "application/x-www-form-urlencoded"
)

// TeamService API docs: https://docs.esa.io/posts/102#4-0-0
type AttachmentService struct {
	client *Client
}

// AttachmentPolicyResponse ファイルアップロードに必要なポリシーのレスポンス
type AttachmentPolicyResponse struct {
	Attachment AttachmentValue `json:"attachment"`
	Form       FormValue       `json:"form"`
}

type AttachmentValue struct {
	Endpoint string `json:"endpoint"`
	Url      string `json:"url"`
}

type FormValue struct {
	AWSAccessKeyId     string `json:"AWSAccessKeyId"`
	Signature          string `json:"signature"`
	Policy             string `json:"policy"`
	Key                string `json:"key"`
	ContentType        string `json:"Content-Type"`
	CacheControl       string `json:"Cache-Control"`
	ContentDisposition string `json:"Content-Disposition"`
	Acl                string `json:"acl"`
}

// getFileType ファイルのMIMEタイプ, サイズ, ベースパスを取得する
func (a *AttachmentService) getFileInfo(path string) (url.Values, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, nil, err
	}

	return url.Values{
		"type": {http.DetectContentType(data)},
		"name": {filepath.Base(path)},
		"size": {fmt.Sprint(len(data))},
	}, data, nil
}

// postAttachmentPolicy AWS S3にアップロードするための情報を取得する
// (beta版の機能でAPIが用意されていない)
func (a *AttachmentService) postAttachmentPolicy(teamName string, values url.Values) (*AttachmentPolicyResponse, error) {
	var attachmentPolicyRes AttachmentPolicyResponse

	teamURL := TeamURL + "/" + teamName + AttchmentPolicyURL
	data := bytes.NewBufferString(values.Encode())

	res, err := a.client.post(teamURL, PolicyBodyType, data, &attachmentPolicyRes)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	return &attachmentPolicyRes, nil
}

// UploadAttachmentFile ファイルをesaにアップロードする
func (a *AttachmentService) UploadAttachmentFile(teamName string, path string) (string, error) {
	var err error

	values, data, err := a.getFileInfo(path)
	if err != nil {
		return "", fmt.Errorf("getFileInfo Failed (path: %s): %v\n", path, err)
	}

	policy, err := a.postAttachmentPolicy(teamName, values)
	if err != nil {
		return "", fmt.Errorf("postAttachmentPolicy Failed (values: %v): %v\n", values, err)
	}

	part := &bytes.Buffer{}
	w := multipart.NewWriter(part)
	defer w.Close()

	w.WriteField("AWSAccessKeyId", policy.Form.AWSAccessKeyId)
	w.WriteField("signature", policy.Form.Signature)
	w.WriteField("policy", policy.Form.Policy)
	w.WriteField("key", policy.Form.Key)
	w.WriteField("Content-Type", policy.Form.ContentType)
	w.WriteField("Cache-Control", policy.Form.CacheControl)
	w.WriteField("Content-Disposition", policy.Form.ContentDisposition)
	w.WriteField("acl", policy.Form.Acl)

	file, err := w.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return "", fmt.Errorf("CreateFormFile Failed: %v\n", err)
	}

	_, err = io.Copy(file, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("Copy Failed: %v\n", err)
	}

	res, err := a.client.Client.Post(policy.Attachment.Endpoint, w.FormDataContentType(), part)
	if err != nil {
		return "", fmt.Errorf("Post Failed (endpoint: %s): %v\n", policy.Attachment.Endpoint, err)
	}
	defer res.Body.Close()

	// ref: https://github.com/esaio/esa-ruby/blob/3431e02e967845cf4c12bbd5860312d7dda2771f/lib/esa/api_methods.rb#L181
	if res.StatusCode != http.StatusNoContent {
		return "", fmt.Errorf("HTTP status is not http.StatusNoContent: %v\n", http.StatusText(res.StatusCode))
	}

	return policy.Attachment.Url, nil
}

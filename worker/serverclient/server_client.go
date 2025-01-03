package serverclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"io"
	"net/http"
	"time"
	"transcoder/model"
	"transcoder/server/web"
)

type ServerClient struct {
	webServerConfig web.WebServerConfig
	httpClient      *http.Client
}

func NewServerClient(webServerConfig web.WebServerConfig) *ServerClient {

	client := retryablehttp.NewClient()
	client.RetryMax = 999999999999
	client.RetryWaitMin = 5 * time.Second
	//client.Logger = nil
	//client.ResponseLogHook = func(l retryablehttp.Logger, r *http.Response) {
	//	log.Warn("Response: ", r.StatusCode)
	//}
	//client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
	//	log.Infof("Retrying request %v", err)
	//	return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	//}

	return &ServerClient{
		webServerConfig: webServerConfig,
		httpClient:      client.StandardClient(),
	}
}

func (Q *ServerClient) PublishEvent(event model.TaskEvent) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", Q.buildURL("/api/v1/event"), bytes.NewBuffer(b))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := Q.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

var NoJobAvailable = errors.New("no job available")

func (Q *ServerClient) RequestJob() (*model.TaskEncode, error) {
	resp, err := Q.httpClient.Get(Q.buildURL("/api/v1/job/request"))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, NoJobAvailable
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(fmt.Sprintf("Error requesting job: %s", resp.Status))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	job := &model.TaskEncode{}
	err = json.Unmarshal(body, job)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (Q *ServerClient) GetDownloadURL(id uuid.UUID) string {
	return Q.buildURLWithParam("/api/v1/download", "uuid", id.String())
}

func (Q *ServerClient) GetChecksumURL(id uuid.UUID) string {
	return Q.buildURLWithParam("/api/v1/checksum", "uuid", id.String())
}

func (Q *ServerClient) GetUploadURL(id uuid.UUID) string {
	return Q.buildURLWithParam("/api/v1/upload", "uuid", id.String())
}

func (Q *ServerClient) buildURL(uri string) string {
	return fmt.Sprintf("%s%s?token=%s", Q.webServerConfig.Domain, uri, Q.webServerConfig.Token)
}

func (Q *ServerClient) buildURLWithParam(uri, k, v string) string {
	return fmt.Sprintf("%s&%s=%s", Q.buildURL(uri), k, v)
}

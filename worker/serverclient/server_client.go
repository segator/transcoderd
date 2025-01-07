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
	client.Logger = nil
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
	req, err := Q.request("POST", "/api/v1/event", bytes.NewBuffer(b))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return err
	}

	resp, err := Q.httpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("Error publishing event: %s", resp.Status))
	}
	defer resp.Body.Close()
	return nil
}

var NoJobAvailable = errors.New("no job available")

func (Q *ServerClient) RequestJob(workerName string) (*model.TaskEncode, error) {
	req, err := Q.request("GET", "/api/v1/job/request", nil)
	req.Header.Set("workerName", workerName)
	if err != nil {
		return nil, err
	}
	resp, err := Q.httpClient.Do(req)
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
	return fmt.Sprintf("%s?uuid=%s", Q.GetURL("/api/v1/download"), id.String())
}

func (Q *ServerClient) GetChecksumURL(id uuid.UUID) string {
	return fmt.Sprintf("%s?uuid=%s", Q.GetURL("/api/v1/checksum"), id.String())
}

func (Q *ServerClient) GetUploadURL(id uuid.UUID) string {
	return fmt.Sprintf("%s?uuid=%s", Q.GetURL("/api/v1/upload"), id.String())
}

func (Q *ServerClient) GetURL(uri string) string {
	return fmt.Sprintf("%s%s", Q.webServerConfig.Domain, uri)
}

func (Q *ServerClient) request(method string, uri string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, Q.GetURL(uri), body)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return nil, err
	}

	authHeader := fmt.Sprintf("Bearer %s", Q.webServerConfig.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)

	return req, nil
}

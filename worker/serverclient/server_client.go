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
	"transcoder/helper"
	"transcoder/model"
	"transcoder/server/web"
)

type ServerClient struct {
	webServerConfig *web.Config
	httpClient      *http.Client
	workerName      string
}

func NewServerClient(webServerConfig *web.Config, workerName string) *ServerClient {

	client := retryablehttp.NewClient()
	client.RetryMax = 999999999999
	client.RetryWaitMin = 5 * time.Second
	client.Logger = nil
	// client.ResponseLogHook = func(l retryablehttp.Logger, r *http.Response) {
	//	log.Warn("Response: ", r.StatusCode)
	// }
	// client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
	//	log.Infof("Retrying request %v", err)
	//	return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	// }

	return &ServerClient{
		webServerConfig: webServerConfig,
		workerName:      workerName,
		httpClient:      client.StandardClient(),
	}
}

func (s *ServerClient) PublishEvent(event model.TaskEvent) error {
	event.WorkerName = s.workerName
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	req, err := s.request("POST", "/api/v1/event", bytes.NewBuffer(b))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("error publishing event: %s", resp.Status)
	}
	defer resp.Body.Close()

	return nil
}

var NoJobAvailable = errors.New("no job available")

func (s *ServerClient) RequestJob(workerName string) (*model.TaskEncode, error) {
	req, err := s.request("GET", "/api/v1/job/request", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("workerName", workerName)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, NoJobAvailable
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error requesting job: %s", resp.Status)
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

func (s *ServerClient) GetDownloadURL(id uuid.UUID) string {
	return fmt.Sprintf("%s?uuid=%s", s.GetURL("/api/v1/download"), id.String())
}

func (s *ServerClient) GetChecksumURL(id uuid.UUID) string {
	return fmt.Sprintf("%s?uuid=%s", s.GetURL("/api/v1/checksum"), id.String())
}

func (s *ServerClient) GetUploadURL(id uuid.UUID) string {
	return fmt.Sprintf("%s?uuid=%s", s.GetURL("/api/v1/upload"), id.String())
}

func (s *ServerClient) GetURL(uri string) string {
	return fmt.Sprintf("%s%s", s.webServerConfig.Domain, uri)
}

func (s *ServerClient) request(method string, uri string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, s.GetURL(uri), body)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return nil, err
	}

	authHeader := fmt.Sprintf("Bearer %s", s.webServerConfig.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)

	return req, nil
}

func (s *ServerClient) PublishPing() error {
	publicIp, err := helper.GetPublicIP()
	if err != nil {
		return err
	}
	pingEvent := model.TaskEvent{
		EventType:  model.PingEvent,
		WorkerName: s.workerName,
		EventTime:  time.Now(),
		IP:         publicIp,
	}
	return s.PublishEvent(pingEvent)
}

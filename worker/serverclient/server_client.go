package serverclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
	"transcoder/model"
	"transcoder/server/web"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

type ServerClient struct {
	webServerConfig     *web.Config
	retriableHttpClient *http.Client
	workerName          string
	httpClient          *http.Client
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
		webServerConfig:     webServerConfig,
		workerName:          workerName,
		retriableHttpClient: client.StandardClient(),
		httpClient:          &http.Client{},
	}
}

func (s *ServerClient) publishEvent(event *model.EnvelopEvent) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	req, err := s.request("POST", "/api/v1/event", bytes.NewBuffer(b))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return err
	}
	var resp *http.Response
	if event.EventType == model.PingEvent || event.EventType == model.ProgressEvent {
		resp, err = s.httpClient.Do(req)
	} else {
		resp, err = s.retriableHttpClient.Do(req)
	}
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("error publishing event: %s", resp.Status)
	}
	defer resp.Body.Close()

	return nil
}

var ErrNoJobAvailable = errors.New("no job available")

func (s *ServerClient) RequestJob() (*model.RequestJobResponse, error) {
	req, err := s.request("GET", "/api/v1/job/request", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("workerName", s.workerName)
	resp, err := s.retriableHttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, ErrNoJobAvailable
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error requesting job: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	job := &model.RequestJobResponse{}
	err = json.Unmarshal(body, job)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *ServerClient) GetURL(uri string) string {
	return fmt.Sprintf("%s%s", s.webServerConfig.Domain, uri)
}

func (s *ServerClient) GetBaseDomain() string {
	return s.webServerConfig.Domain
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

func (s *ServerClient) PublishPingEvent() error {
	pingEvent := model.PingEventType{
		Event: model.Event{
			EventTime:  time.Now(),
			WorkerName: s.workerName,
		},
	}
	event, err := envelopEvent(model.PingEvent, pingEvent)
	if err != nil {
		return err
	}

	return s.publishEvent(event)
}

func (s *ServerClient) PublishTaskEvent(taskEvent *model.TaskEventType) error {
	taskEvent.WorkerName = s.workerName
	event, err := envelopEvent(model.NotificationEvent, taskEvent)
	if err != nil {
		return err
	}

	return s.publishEvent(event)
}

func (s *ServerClient) PublishTaskProgressEvent(taskProgress *model.TaskProgressType) error {
	taskProgress.WorkerName = s.workerName
	event, err := envelopEvent(model.ProgressEvent, taskProgress)
	if err != nil {
		return err
	}

	return s.publishEvent(event)
}

func envelopEvent(eventType model.EventType, eventData interface{}) (*model.EnvelopEvent, error) {
	b, err := json.Marshal(eventData)
	if err != nil {
		return nil, err
	}
	return &model.EnvelopEvent{
		EventType: eventType,
		EventData: b,
	}, nil
}

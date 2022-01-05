package client

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"k8s.io/apimachinery/pkg/util/json"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	"github.com/fabedge/fab-dns/pkg/service-hub/apiserver"
)

const defaultTimeout = 5 * time.Second

type HttpError struct {
	Response *http.Response
	Message  string
}

func (e HttpError) Error() string {
	return fmt.Sprintf("Status Code: %d. Message: %s", e.Response.StatusCode, e.Message)
}

type Interface interface {
	Heartbeat() error
	UploadGlobalService(service apis.GlobalService) error
	DownloadAllGlobalServices() ([]apis.GlobalService, error)
	DeleteGlobalService(namespace, name string) error
}

var _ Interface = &client{}

type client struct {
	clusterName string
	baseURL     *url.URL
	httpClient  *http.Client
}

func NewClient(apiServerAddr string, clusterName string, transport http.RoundTripper) (Interface, error) {
	baseURL, err := url.Parse(apiServerAddr)
	if err != nil {
		return nil, err
	}

	return &client{
		baseURL:     baseURL,
		clusterName: clusterName,
		httpClient: &http.Client{
			Timeout:   defaultTimeout,
			Transport: transport,
		},
	}, nil
}

func (c *client) Heartbeat() error {
	req, err := http.NewRequest(http.MethodGet, join(c.baseURL, apiserver.PathHeartbeat), nil)
	if err != nil {
		return err
	}
	req.Header.Set(apiserver.HeaderClusterName, c.clusterName)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	_, err = handleResponse(resp)
	return err
}

func (c *client) UploadGlobalService(service apis.GlobalService) error {
	data, err := json.Marshal(service)
	if err != nil {
		return err
	}

	url := join(c.baseURL, apiserver.PathGlobalServices)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(apiserver.HeaderClusterName, c.clusterName)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	_, err = handleResponse(resp)
	return err
}

func (c *client) DownloadAllGlobalServices() (services []apis.GlobalService, err error) {
	req, err := http.NewRequest(http.MethodGet, join(c.baseURL, apiserver.PathGlobalServices), nil)
	if err != nil {
		return services, err
	}
	req.Header.Set(apiserver.HeaderClusterName, c.clusterName)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return services, err
	}

	data, err := handleResponse(resp)

	err = json.Unmarshal(data, &services)
	return services, err
}

func (c *client) DeleteGlobalService(namespace, name string) error {
	addr := fmt.Sprintf("%s/%s/%s", apiserver.PathGlobalServices, namespace, name)
	req, err := http.NewRequest(http.MethodDelete, join(c.baseURL, addr), nil)
	if err != nil {
		return err
	}
	req.Header.Set(apiserver.HeaderClusterName, c.clusterName)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	_, err = handleResponse(resp)
	return err
}

func join(baseURL *url.URL, ref string) string {
	u, _ := baseURL.Parse(ref)
	return u.String()
}

func handleResponse(resp *http.Response) (content []byte, err error) {
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		content, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}

		return nil, &HttpError{
			Response: resp,
			Message:  string(content),
		}
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	return ioutil.ReadAll(resp.Body)
}

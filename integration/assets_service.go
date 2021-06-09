package integration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type AssetsService interface {
	Doc2Text(uid string) (string, error)
	Prepare(uids []string) (bool, map[string]int, error)
}

type DefaultAssetsService struct {
	client  *http.Client
	baseUrl string
}

func NewAssetsService(url string) AssetsService {
	return &DefaultAssetsService{
		baseUrl: url,
		client: &http.Client{
			Timeout: 600 * time.Second,
		},
	}
}

func (s *DefaultAssetsService) Doc2Text(uid string) (string, error) {
	resp, err := s.client.Get(fmt.Sprintf("%s/doc2text/%s", s.baseUrl, uid))

	if err != nil {
		return "", errors.Wrap(err,"client.Get")
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("%d %s", resp.StatusCode , resp.Status)
	}

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err,"ioutil.ReadAll")
	}

	return string(bodyBytes), nil
}


type unzipResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *DefaultAssetsService) Prepare(uids []string) (bool, map[string]int, error) {
	resp, err := s.client.Get(fmt.Sprintf("%s/prepare/%s", s.baseUrl, strings.Join(uids, ",")))
	if err != nil {
		return false, nil, errors.Wrap(err,"client.Get")
	}
	if resp.StatusCode != http.StatusOK {
		return false, nil, errors.Errorf("%d %s", resp.StatusCode , resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, nil, errors.Wrap(err,"ioutil.ReadAll")
	}

	var data []unzipResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return false, nil, errors.Wrap(err,"json.Unmarshal")
	}

	if len(data) != len(uids) {
		return false, nil, errors.Errorf("Response length mismatch. Expected %d, got %d", len(uids), len(data))
	}

	backoff := false
	successMap := make(map[string]int, len(uids))
	var errs []string
	for i, subResponse := range data {
		successMap[uids[i]] = subResponse.Code
		if subResponse.Code == http.StatusServiceUnavailable {
			backoff = true
		} else if subResponse.Code != http.StatusOK {
			// Don't repeat request, continue.
			errs = append(errs, subResponse.Message)
		}
	}
	//if len(errs) > 0 {
	//	log.Warn(strings.Join(errors, ","))
	//}

	return backoff, successMap, nil
}

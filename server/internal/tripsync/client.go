package tripsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseSize = 16 << 20

type cloudError struct {
	Status  int
	Message string
}

func (e *cloudError) Error() string { return e.Message }

// StatusOf exposes the cloud HTTP status for user-facing error mapping.
func StatusOf(err error) (int, bool) {
	var target *cloudError
	if errors.As(err, &target) {
		return target.Status, true
	}
	return 0, false
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func endpoint(baseURL, path string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("云端地址无效")
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawPath = ""
	return u.String(), nil
}

func request(ctx context.Context, method, baseURL, path string, query url.Values, body any) (json.RawMessage, error) {
	target, err := endpoint(baseURL, path)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(target)
	if err != nil {
		return nil, errors.New("云端请求无效")
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, errors.New("云端请求无效")
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, errors.New("云端请求无效")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("连接云端失败或超时")
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil || len(payload) > maxResponseSize {
		return nil, errors.New("云端响应读取失败或过大")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var decoded envelope
		message := "云端请求失败"
		if json.Unmarshal(payload, &decoded) == nil && strings.TrimSpace(decoded.Message) != "" {
			message = decoded.Message
		}
		return nil, &cloudError{Status: resp.StatusCode, Message: message}
	}
	var decoded envelope
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, errors.New("云端响应格式错误")
	}
	if decoded.Code != 200 {
		message := strings.TrimSpace(decoded.Message)
		if message == "" {
			message = "云端业务响应失败"
		}
		return nil, &cloudError{Status: http.StatusBadGateway, Message: message}
	}
	return decoded.Data, nil
}

func FetchCandidates(ctx context.Context, baseURL, siteCode, plate string) ([]Trip, error) {
	data, err := request(ctx, http.MethodGet, baseURL, "/api/v1/weighbridge/sync/trips", url.Values{"siteCode": {siteCode}, "plate": {plate}}, nil)
	if err != nil {
		return nil, err
	}
	var trips []Trip
	if err := json.Unmarshal(data, &trips); err != nil || trips == nil {
		return nil, errors.New("云端流转单格式错误")
	}
	for i := range trips {
		if trips[i].Deductions == nil {
			trips[i].Deductions = []Deduction{}
		}
		if trips[i].Blockers == nil {
			trips[i].Blockers = []string{}
		}
	}
	return trips, nil
}

func FetchTrip(ctx context.Context, baseURL, siteCode, tripNo string) (Trip, error) {
	if strings.TrimSpace(tripNo) == "" {
		return Trip{}, errors.New("流转单号不能为空")
	}
	data, err := request(ctx, http.MethodGet, baseURL, "/api/v1/weighbridge/sync/trips/"+url.PathEscape(tripNo), url.Values{"siteCode": {siteCode}}, nil)
	if err != nil {
		return Trip{}, err
	}
	var trip Trip
	if err := json.Unmarshal(data, &trip); err != nil || trip.TripNo == "" {
		return Trip{}, errors.New("云端流转单格式错误")
	}
	if trip.Deductions == nil {
		trip.Deductions = []Deduction{}
	}
	if trip.Blockers == nil {
		trip.Blockers = []string{}
	}
	return trip, nil
}

func LinkTicket(ctx context.Context, baseURL, siteCode, tripNo, ticketNo string) error {
	if tripNo == "" || ticketNo == "" {
		return errors.New("流转单号和磅单号不能为空")
	}
	_, err := request(ctx, http.MethodPost, baseURL, "/api/v1/weighbridge/sync/trips/"+url.PathEscape(tripNo)+"/link", nil, map[string]string{"siteCode": siteCode, "ticketNo": ticketNo})
	return err
}

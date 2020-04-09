package rtcache

import (
	"bytes"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"
)

const NoCacheHeader = "X-No-Cache"

type rtCache struct {
	syncMap  *sync.Map
	wrap     http.RoundTripper
	expireIn time.Duration
}

type responseCache struct {
	*http.Response
	expireAt time.Time
	content  []byte
}

func (rc responseCache) ToResponse() *http.Response {

	return &http.Response{
		Status:           rc.Response.Status,
		StatusCode:       rc.Response.StatusCode,
		Proto:            rc.Response.Proto,
		ProtoMajor:       rc.Response.ProtoMajor,
		ProtoMinor:       rc.Response.ProtoMinor,
		Header:           rc.Response.Header.Clone(),
		Body:             ioutil.NopCloser(bytes.NewBuffer(rc.content)),
		ContentLength:    rc.Response.ContentLength,
		TransferEncoding: rc.Response.TransferEncoding,
		Close:            rc.Response.Close,
		Uncompressed:     rc.Response.Uncompressed,
		Trailer:          rc.Response.Trailer,
		Request:          rc.Response.Request,
		TLS:              rc.Response.TLS,
	}
}

type options func(rt *rtCache)

func SetWrapRoundTripper(wrap http.RoundTripper) options {
	return func(rt *rtCache) {
		rt.wrap = wrap
	}
}

func NewRoundTripperCache(expireIn time.Duration, opts ...options) *rtCache {
	rt := &rtCache{
		syncMap:  &sync.Map{},
		expireIn: expireIn,
		wrap: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(rt)
	}
	return rt
}

func (r rtCache) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return r.wrap.RoundTrip(req)
	}
	urlKey := req.URL.String()
	respRaw, ok := r.syncMap.Load(urlKey)
	if ok &&
		respRaw.(*responseCache).expireAt.After(time.Now()) &&
		req.Header.Get(NoCacheHeader) == "" {
		return respRaw.(*responseCache).ToResponse(), nil
	}

	resp, err := r.wrap.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	body := resp.Body
	defer body.Close()
	if resp.StatusCode > 299 {
		return resp, err
	}

	content := make([]byte, 0)
	if body != nil {
		content, err = ioutil.ReadAll(body)
		if err != nil {
			return resp, err
		}
	}

	resp.Body = ioutil.NopCloser(bytes.NewBuffer(content))
	finalResp := &responseCache{
		Response: resp,
		expireAt: time.Now().Add(r.expireIn),
		content:  content,
	}
	r.syncMap.Store(urlKey, finalResp)
	return finalResp.Response, nil
}

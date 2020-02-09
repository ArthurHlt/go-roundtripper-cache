package rtcache

import (
	"bytes"
	"io"
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

func newBufReadCloser(r io.Reader) (*bufReadCloser, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &bufReadCloser{bytes.NewReader(b)}, nil
}

type bufReadCloser struct {
	*bytes.Reader
}

func (b bufReadCloser) Close() error {
	_, err := b.Seek(0, 0)
	return err
}

type responseCache struct {
	*http.Response
	expireAt time.Time
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
		return respRaw.(*responseCache).Response, nil
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
	buf, err := newBufReadCloser(body)
	if err != nil {
		return nil, err
	}
	resp.Body = buf
	finalResp := &responseCache{
		Response: resp,
		expireAt: time.Now().Add(r.expireIn),
	}
	r.syncMap.Store(urlKey, finalResp)
	return finalResp.Response, nil
}

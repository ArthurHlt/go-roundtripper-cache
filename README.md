# go-roundtripper-cache

Small lib for hooking http client roundtripper for caching GET requests with no error (status code < 299) 
for a defined times.

This was made for making easy lazy caching on multiple api calls on a target. 
This reduce time for next calls and also numbers of calls to the target.

## Usage

```go
package main

import (
    "net/http"
    "time"
    "github.com/ArthurHlt/go-roundtripper-cache"
)

func main() {
    client := http.Client{
        Transport: rtcache.NewRoundTripperCache(24*time.Hour),
    }
    // you can also set your own transport with:
    rtcache.NewRoundTripperCache(24*time.Hour, rtcache.SetWrapRoundTripper(&http.Transport{}))
    
    // now use client as you will normaly do, get request with no error will be cached
    
    // You can force to use no cache on some request by using header X-No-Cache
    req, _ := http.NewRequest("GET", "http://mysite.com", nil)
    req.Header.Add(rtcache.NoCacheHeader, "true")
}
```
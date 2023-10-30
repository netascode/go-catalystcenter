[![Tests](https://github.com/netascode/go-catalystcenter/actions/workflows/test.yml/badge.svg)](https://github.com/netascode/go-catalystcenter/actions/workflows/test.yml)

# go-catalystcenter

`go-catalystcenter` is a Go client library for Cisco Catalyst Center. It is based on Nathan's excellent [goaci](https://github.com/brightpuddle/goaci) module and features a simple, extensible API and [advanced JSON manipulation](#result-manipulation).

## Getting Started

### Installing

To start using `go-catalystcenter`, install Go and `go get`:

`$ go get -u github.com/netascode/go-catalystcenter`

### Basic Usage

```go
package main

import "github.com/netascode/go-catalystcenter"

func main() {
    client, _ := cc.NewClient("https://1.1.1.1", "user", "pwd")

    res, _ := client.Get("/dna/intent/api/v2/site")
    println(res.Get("response.0.name").String())
}
```

This will print something like:

```
Site1
```

#### Result manipulation

`cc.Result` uses GJSON to simplify handling JSON results. See the [GJSON](https://github.com/tidwall/gjson) documentation for more detail.

```go
res, _ := client.Get("/dna/intent/api/v2/site")

for _, site := range res.Get("response").Array() {
    println(site.Get("@pretty").String()) // pretty print sites
}
```

#### POST data creation

`cc.Body` is a wrapper for [SJSON](https://github.com/tidwall/sjson). SJSON supports a path syntax simplifying JSON creation.

```go
body := cc.Body{}.
    Set("type", "area").
    Set("site.area.name", "Area1").
    Set("site.area.parentName", "Global")
client.Post("/dna/intent/api/v1/site", body.Str)
```

## Documentation

See the [documentation](https://godoc.org/github.com/netascode/go-catalystcenter) for more details.

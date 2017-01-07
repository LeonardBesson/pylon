# Pylon [![Build Status](https://travis-ci.org/LeonardBesson/pylon.svg?branch=master)](https://travis-ci.org/LeonardBesson/pylon)
**A Go Reverse Proxy and Load balancer**

## Usage
You just need a config, for example:
```json
{
  "servers": [
    {
      "name": "server1",
      "port": 7777,
      "services": [
        {
          "route_prefix": "/microservice/",
          "instances": [
            {
              "host": "127.0.0.1:1111",
              "weight": 3
            },
            {
              "host": "127.0.0.1:2222"
            },
            {
              "host": "127.0.0.1:3333"
            }
          ],
          "balancing_strategy": "round_robin",
          "max_connections": 300
        }
      ]
    }
  ]
}
```

And simply use it like that:
```go
package main

import (
	"github.com/leonardbesson/pylon"
	"log"
)

func main() {
	log.Fatal(pylon.ListenAndServe("./config.json"))
}
```

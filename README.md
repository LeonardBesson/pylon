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
      "monitoring_route": "/health",
      "services": [
        {
          "name": "Billing Service",
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
          "max_connections": 300,
          "health_check": {
            "enabled": true,
            "interval": 30,
            "dial_timeout": 2
          }
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
	"os"
)

func main() {
       f, err := os.OpenFile("pylon.log", os.O_RDWR | os.O_CREATE | os.O_TRUNC, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	
       // Redirect the logging to a file
	pylon.SetLogWriter(f)
	// Set the logging levels
	pylon.SetLogLevels(pylon.LOG_ERROR | pylon.LOG_INFO)
	// Specify a custom logging function (log.Println is already the default)
	pylon.SetInfoLogger(log.Println)
	pylon.SetErrorLogger(log.Println)
	
	log.Fatal(pylon.ListenAndServe("./config.json"))
}
```
## Executables
If you don't want to use Pylon in your own library or main, I have put pre-compiled executables (Win x64 and Linux x64) running a simple main in the "Release" section.

Launch it with the config file named "config.json" in the same directory and it will create a log file named "pylon.log". You can also specify the config and log files, for example:
```
 ./pylon_linux_x64 -c /path/to/config.json -log /your/log/file
```

package pylon

import (
	"testing"
	"strings"
	"reflect"
	"log"
)

func TestJSONConfigParser_Parse(t *testing.T) {
	const conf = `  {
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
			}`

	expected := &Config{
		[]Server{{
			"server1",
			7777,
			[]Service{{
				"",
				"/microservice/",
				[]Instance{
					{"127.0.0.1:1111", 3, nil, 0},
					{"127.0.0.1:2222", 0, nil, 0},
					{"127.0.0.1:3333", 0, nil, 0},
				},
				RoundRobin,
				300,
				HealthCheck{false, 0},
			}},
		}},
	}

	reader := strings.NewReader(conf)
	parser := &JSONConfigParser{}
	in, err := parser.Parse(reader)
	if err != nil {
		t.FailNow()
	}
	if !reflect.DeepEqual(in, expected) {
		log.Printf("\nExpected %#v\nbut got  %#v", in, expected)
		t.Fail()
	}
}

func TestValidateConfig(t *testing.T) {
	right := &Config{
		[]Server{{
			"server1",
			7777,
			[]Service{{
				"",
				"/microservice/",
				[]Instance{
					{"127.0.0.1:1111", 3, nil, 0},
					{"127.0.0.1:2222", 0, nil, 0},
					{"127.0.0.1:3333", 0, nil, 0},
				},
				RoundRobin,
				300,
				HealthCheck{false, 0},
			}},
		}},
	}

	wrong := &Config{
		[]Server{{
			"server1",
			7777,
			[]Service{{
				"**/regex_route",
				"",
				[]Instance{
					{"127.0.0.1:1111", 3, nil, 0},
					{"127.0.0.1:2222", 0, nil, 0},
					{"127.0.0.1:3333", 0, nil, 0},
				},
				"wrong_strategy",
				0,
				HealthCheck{false, 0},
			}},
		}},
	}

	if err := validateConfig(right); err != nil {
		t.Log(err.Error())
		t.Fail()
	}

	if err := validateConfig(wrong); err == nil {
		t.Log(err.Error())
		t.Fail()
	}
}

func TestValidateServer(t *testing.T) {
	right := &Server{
		"server1",
		7777,
		[]Service{{
			"",
			"/microservice/",
			[]Instance{
				{"127.0.0.1:1111", 3, nil, 0},
				{"127.0.0.1:2222", 0, nil, 0},
				{"127.0.0.1:3333", 0, nil, 0},
			},
			RoundRobin,
			300,
			HealthCheck{false, 0},
		}},
	}

	wrong := &Server{
		"server1",
		-1,
		[]Service{{
			"**/regex_route",
			"",
			[]Instance{
				{"12.0.1:1111", 3, nil, 0},
				{"127.0.0.1:2222", 0, nil, 0},
				{"127.0.0.1:3333", 0, nil, 0},
			},
			"wrong_strategy",
			0,
			HealthCheck{false, 0},
		}},
	}

	if err := validateServer(right); err != nil {
		t.Log(err.Error())
		t.Fail()
	}

	if err := validateServer(wrong); err == nil {
		t.Log(err.Error())
		t.Fail()
	}
}
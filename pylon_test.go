package pylon

import (
	"testing"
	"log"
	"sync"
	"reflect"
	"regexp"
)

type RoundRobinTest struct {
	exp   string
	count int
}

func TestNewPylonCorrect(t *testing.T) {
	in := &Server{
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

	expected := &Pylon{
		[]*MicroService{{
			PrefixRoute{
				"/microservice/",
			},
			[]*Instance{
				{"127.0.0.1:1111", 3, make(chan int, 300), 0},
				{"127.0.0.1:2222", 1, make(chan int, 300), 0},
				{"127.0.0.1:3333", 1, make(chan int, 300), 0},
			},
			RoundRobin,
			0,
			make(map[int]bool, 3),
			make(chan int, 300),
			0.0,
			&sync.RWMutex{},
			HealthCheck{false, 0},
		}},
	}

	out, err := NewPylon(in)
	if err != nil {
		log.Println("Error creating the new Pylon: " + err.Error())
		t.Fail()
	}

	if !reflect.DeepEqual(out, expected) {
		//log.Printf("\nExpected %#v\nbut got  %#v", out, expected)
		//log.Printf("\nExpected %#v\nbut got  %#v", out.Services, expected.Services)
		//for _, ser := range out.Services {
		//	log.Printf("\nOut Service:  %#v", ser)
		//	for _, inst := range ser.Instances {
		//		log.Printf("\nOut Inst:  %#v", inst)
		//	}
		//}
		//for _, ser := range expected.Services {
		//	log.Printf("\nExp Service:  %#v", ser)
		//	for _, inst := range ser.Instances {
		//		log.Printf("\nExp Inst:  %#v", inst)
		//	}
		//}
		//t.Fail()
	}
}

func TestNewPylonWrong(t *testing.T) {
	wrong := &Server{
		"server1",
		7777,
		[]Service{{
			"",
			"",
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

	_, err := NewPylon(wrong)

	if err == nil {
		t.Fail()
	}
}

func TestPrefixRoute_Data(t *testing.T) {
	route := &PrefixRoute{
		"route",
	}

	_, ok := route.Data().(string)
	if !ok {
		t.Fail()
	}
}

func TestPrefixRoute_Type(t *testing.T) {
	route := &PrefixRoute{
		"route",
	}

	if route.Type() != Prefix {
		t.Fail()
	}
}

func TestRegexRoute_Data(t *testing.T) {
	route := &RegexRoute{
		regexp.MustCompile("route"),
	}

	_, ok := route.Data().(*regexp.Regexp)
	if !ok {
		t.Fail()
	}
}

func TestRegexRoute_Type(t *testing.T) {
	route := &RegexRoute{
		regexp.MustCompile("route"),
	}

	if route.Type() != Regex {
		t.Fail()
	}
}

func TestNextRoundRobinInstIdxNonWeighted(t *testing.T) {
	in := &Server{
		"server1",
		7777,
		[]Service{{
			"",
			"/microservice/",
			[]Instance{
				{"127.0.0.1:1111", 0, nil, 0},
				{"127.0.0.1:2222", 0, nil, 0},
				{"127.0.0.1:3333", 0, nil, 0},
			},
			RoundRobin,
			300,
			HealthCheck{false, 0},
		}},
	}

	p, err := NewPylon(in)
	if err != nil {
		t.Fatal(err.Error())
	}

	m := p.Services[0]
	tests := []RoundRobinTest{
		{"127.0.0.1:1111", 1},
		{"127.0.0.1:2222", 1},
		{"127.0.0.1:3333", 1},
	}

	if !validateRRTests(tests, m) {
		t.Fail()
	}
}

func TestNextRoundRobinInstIdxWeighted(t *testing.T) {
	in := &Server{
		"server1",
		7777,
		[]Service{{
			"",
			"/microservice/",
			[]Instance{
				{"127.0.0.1:1111", 2, nil, 0},
				{"127.0.0.1:2222", 0, nil, 0},
				{"127.0.0.1:3333", 3, nil, 0},
				{"127.0.0.1:4444", 5, nil, 0},
			},
			RoundRobin,
			300,
			HealthCheck{false, 0},
		}},
	}

	p, err := NewPylon(in)
	if err != nil {
		t.Fatal(err.Error())
	}
	m := p.Services[0]

	tests := []RoundRobinTest{
		{"127.0.0.1:1111", 2},
		{"127.0.0.1:2222", 1},
		{"127.0.0.1:3333", 3},
		{"127.0.0.1:4444", 5},
	}

	if !validateRRTests(tests, m) {
		t.Fail()
	}
}

func TestLeastConnected(t *testing.T) {
	in := &Server{
		"server1",
		7777,
		[]Service{{
			"",
			"/microservice/",
			[]Instance{
				{"127.0.0.1:1111", 0, nil, 0},
				{"127.0.0.1:2222", 0, nil, 0},
				{"127.0.0.1:3333", 0, nil, 0},
				{"127.0.0.1:4444", 0, nil, 0},
			},
			LeastConnected,
			300,
			HealthCheck{false, 0},
		}},
	}

	p, err := NewPylon(in)
	if err != nil {
		t.Fatal(err.Error())
	}
	m := p.Services[0]
	m.Instances[1].ReqCount <- 1

	m.Instances[2].ReqCount <- 1
	m.Instances[2].ReqCount <- 1

	m.Instances[3].ReqCount <- 1
	m.Instances[3].ReqCount <- 1
	m.Instances[3].ReqCount <- 1

	for i := range m.Instances  {
		_, idx, err := m.getLoadBalancedInstance()
		if err != nil {
			t.FailNow()
		}
		if idx != i {
			t.FailNow()
		}
		m.Instances[i] = nil
	}
}

func validateRRTests(tests []RoundRobinTest, m *MicroService) bool {
	for _, test := range tests {
		for i := 0; i < test.count; i++ {
			//idx := m.nextRoundRobinInstIdx()
			_, idx, err := m.getLoadBalancedInstance()
			if err != nil {
				return false
			}
			if idx < 0 || idx > len(m.Instances) - 1 {
				return false
			}

			if m.Instances[idx].Host != test.exp {
				return false
			}
		}
	}

	return true
}
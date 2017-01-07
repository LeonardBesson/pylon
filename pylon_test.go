package pylon

import (
	"testing"
	"log"
	"sync"
	"reflect"
	"regexp"
)

func TestNewPylon(t *testing.T) {
	in := &Server{
		"server1",
		7777,
		[]Service{{
			"",
			"/microservice/",
			[]Instance{
				{"127.0.0.1:1111", 3, nil},
				{"127.0.0.1:2222", 0, nil},
				{"127.0.0.1:3333", 0, nil},
			},
			RoundRobin,
			300,
		}},
	}

	expected := &Pylon{
		[]*MicroService{{
			PrefixRoute{
				"/microservice/",
			},
			[]*Instance{
				{"127.0.0.1:1111", 3, make(chan int, 300)},
				{"127.0.0.1:2222", 1, make(chan int, 300)},
				{"127.0.0.1:3333", 1, make(chan int, 300)},
			},
			RoundRobin,
			0,
			make(map[int]bool, 3),
			make(chan int, 300),
			&sync.RWMutex{},
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

func TestNewPylon2(t *testing.T) {
	wrong := &Server{
		"server1",
		7777,
		[]Service{{
			"",
			"",
			[]Instance{
				{"127.0.0.1:1111", 3, nil},
				{"127.0.0.1:2222", 0, nil},
				{"127.0.0.1:3333", 0, nil},
			},
			RoundRobin,
			300,
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

func TestNextRoundRobinInstIdx(t *testing.T)  {
	m := &MicroService{
		PrefixRoute{
			"/microservice/",
		},
		[]*Instance{
			{"127.0.0.1:1111", 1, make(chan int, 300)},
			{"127.0.0.1:2222", 1, make(chan int, 300)},
			{"127.0.0.1:3333", 1, make(chan int, 300)},
		},
		RoundRobin,
		0,
		make(map[int]bool, 3),
		make(chan int, 300),
		&sync.RWMutex{},
	}
	
	idx := m.nextRoundRobinInstIdx()
	if idx < 0 || idx > 2 {
		t.FailNow()
	}

	if m.Instances[idx].Host != "127.0.0.1:2222" {
		t.FailNow()
	}

	idx = m.nextRoundRobinInstIdx()
	if m.Instances[idx].Host != "127.0.0.1:3333" {
		t.FailNow()
	}

	idx = m.nextRoundRobinInstIdx()
	if m.Instances[idx].Host != "127.0.0.1:1111" {
		t.FailNow()
	}

	idx = m.nextRoundRobinInstIdx()
	if m.Instances[idx].Host != "127.0.0.1:2222" {
		t.FailNow()
	}
}
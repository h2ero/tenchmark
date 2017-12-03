package main

import (
	"fmt"
	"runtime"
	"sync"

	. "github.com/stdrickforce/thriftgo/protocol"
	. "github.com/stdrickforce/thriftgo/transport"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	wg  sync.WaitGroup
	wg2 sync.WaitGroup
)

var (
	requests    = kingpin.Flag("requests", "Number of requests to perform").Short('n').Default("1000").Int()
	concurrency = kingpin.Flag("concurrency", "Number of multiple requests to make at a time").Short('c').Default("10").Int()
	protocol    = kingpin.Flag("protocol", "Specify protocol factory").Default("binary").String()
	transport   = kingpin.Flag("transport", "Specify transport factory").Default("socket").String()
	wrapper     = kingpin.Flag("wrapper", "Specify transport wrapper").Default("buffered").String()
	service     = kingpin.Flag("service", "Specify service name").String()

	thrift_file = kingpin.Flag("thrift-file", "Path to thrift file").Short('f').String()
	api_file    = kingpin.Flag("api-file", "Path to api file").String()
	case_name   = kingpin.Flag("case", "Specify case name").Default("").String()

	addr = kingpin.Arg("addr", "Server addr").Default(":6000").String()
)

func get_transport_wrapper(name string) TransportWrapper {
	switch name {
	case "buffered":
		return NewTBufferedTransportFactory(4096, 4096)
	case "framed":
		return NewTFramedTransportFactory(false, true)
	default:
		panic("invalid transport wrapper")
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	kingpin.Parse()

	if *concurrency <= 0 {
		panic("Invalid number of concurrency")
	}

	if *requests <= 0 {
		panic("Invalid number of requests")
	}

	*service = "Revenue"

	var processor = &Processor{
		chSuccess: make(chan int, *concurrency*2),
		chError:   make(chan int32, *concurrency*2),
		pf:        NewTBinaryProtocolFactory(true, true),
		tf:        NewTSocketFactory(*addr),
		tw:        NewTBufferedTransportFactory(4096, 4096),

		service:     *service,
		thrift_file: *thrift_file,
		api_file:    *api_file,
		case_name:   *case_name,
	}

	if err := processor.initMessage(); err != nil {
		panic(err)
	}

	if err := processor.test(); err != nil {
		panic(err)
	}

	var pipe = make(chan int, *concurrency)

	fmt.Println("This is Tenchmark, Version 0.1")
	fmt.Println("Copyright 2017 Terence Fan, Baixing, https://github.com/baixing")
	fmt.Println("Licensed under the MIT\n")

	fmt.Printf("Benchmarking %v (be patient)......\n", *addr)

	for i := 0; i < *concurrency; i++ {
		go processor.process(i, pipe)
		wg.Add(1)
	}
	go collect(processor.chSuccess, processor.chError)

	for i := 0; i < *requests; i++ {
		pipe <- i
	}
	close(pipe)
	wg.Wait()

	close(processor.chSuccess)
	close(processor.chError)
	wg2.Wait()
}

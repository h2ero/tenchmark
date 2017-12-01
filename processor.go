package main

import (
	"fmt"
	"math"
	"time"
	"xparser"

	. "github.com/stdrickforce/thriftgo/protocol"
	. "github.com/stdrickforce/thriftgo/thrift"
	. "github.com/stdrickforce/thriftgo/transport"
)

type Processor struct {
	service   string
	pf        ProtocolFactory
	tf        TransportFactory
	tw        TransportWrapper
	chSuccess chan int
	chError   chan int32

	thrift_file string
	api_file    string
	case_name   string
	message     []byte
}

func (p *Processor) process(gid int, pipe <-chan int) {
	defer wg.Done()

	trans := p.tf.GetTransport()
	if err := trans.Open(); err != nil {
		panic(err)
	}
	defer trans.Close()

	for _ = range pipe {
		snano := time.Now().UnixNano()
		if err := p.writeMessage(trans); err != nil {
			fmt.Println(gid, err)
			return
		}
		duration := time.Now().UnixNano() - snano
		p.chSuccess <- int(duration / 1000)
	}
}

func (processor *Processor) collectError(pipe chan<- string) {
	defer close(pipe)

	var (
		count        int
		distribution = make(map[int32]int)
	)

	for mtype := range processor.chError {
		count++
		distribution[mtype]++
	}
	var s = func(k int32) string {
		switch k {
		case ExceptionUnknown:
			return "ExceptionUnknown"
		case ExceptionUnknownMethod:
			return "ExceptionUnknownMethod"
		case ExceptionInvalidMessageType:
			return "ExceptionInvalidMessageType"
		case ExceptionWrongMethodName:
			return "ExceptionWrongMethodName"
		case ExceptionBadSequenceID:
			return "ExceptionBadSequenceID"
		case ExceptionMissingResult:
			return "ExceptionMissingResult"
		case ExceptionInternalError:
			return "ExceptionInternalError"
		case ExceptionProtocolError:
			return "ExceptionProtocolError"
		default:
			return fmt.Sprintf("%d", k)
		}
	}

	if count > 0 {
		pipe <- fmt.Sprintf("Count of the exception replied by server:")
		for mtype, val := range distribution {
			pipe <- fmt.Sprintf("%-32s%d", s(mtype), val)
		}
	}
}

func (processor *Processor) collectSuccess(pipe chan<- string) {
	defer close(pipe)

	snano := time.Now().UnixNano()

	var (
		s     = make([]int, 0)
		count = 0
	)

	for duration := range processor.chSuccess {
		count++
		if count%1000 == 0 {
			pipe <- fmt.Sprintf("Completed %d requests", count)
		}
		s = append(s, duration)
	}
	pipe <- fmt.Sprintf("Finished %d requests", count)
	pipe <- ""

	dnano := time.Now().UnixNano() - snano

	l := len(s)
	sort(s, 0, l-1)

	v := func(denominator int) float64 {
		if denominator <= 0 {
			return float64(s[l-1]) / 1000
		} else {
			return float64(s[l*(denominator-1)/denominator-1]) / 1000
		}
	}

	var (
		duration = float64(dnano) / math.Pow(10, 9)
		qps      = float64(l) / duration
	)

	pipe <- fmt.Sprintf("%-24s%s", "Server Address:", *addr)
	pipe <- ""
	pipe <- fmt.Sprintf("%-24s%d", "Concurrency level:", *concurrency)
	pipe <- fmt.Sprintf("%-24s%.3f seconds", "Time taken for tests:", duration)
	pipe <- fmt.Sprintf("%-24s%d", "Complete requests:", l)
	pipe <- fmt.Sprintf("%-24s%d", "Failed requests:", *requests-l)
	pipe <- fmt.Sprintf("%-24s%.2f [#/sec] (mean)", "Request per second:", qps)
	pipe <- ""

	if l == 0 {
		return
	}

	pipe <- "Percentage of the requests served within a certain time (ms)"
	pipe <- fmt.Sprintf("%4d%% %8.2f", 50, v(2))
	pipe <- fmt.Sprintf("%4d%% %8.2f", 66, v(3))
	pipe <- fmt.Sprintf("%4d%% %8.2f", 75, v(4))
	pipe <- fmt.Sprintf("%4d%% %8.2f", 80, v(5))
	pipe <- fmt.Sprintf("%4d%% %8.2f", 90, v(10))
	pipe <- fmt.Sprintf("%4d%% %8.2f", 95, v(20))
	pipe <- fmt.Sprintf("%4d%% %8.2f", 98, v(50))
	pipe <- fmt.Sprintf("%4d%% %8.2f", 99, v(100))
	pipe <- fmt.Sprintf("%4d%% %8.2f (longest request)", 100, v(-1))
	pipe <- ""
}

func (p *Processor) flushToTrans(trans Transport, skip_result bool) (err error) {
	trans = p.tw.GetTransport(trans)
	proto := p.pf.GetProtocol(trans)

	if p.service != "" {
		proto = NewMultiplexedProtocol(proto, p.service)
	}

	// thrift parser
	parser_instance, err := xparser.InitParser(p.thrift_file)
	if err != nil {
		return
		//panic(err)
	}

	// api case
	var api_case *xparser.APICase
	if p.case_name == "" {
		if api_case, err = xparser.GetPingCase(); err != nil {
			return
		}
	} else if api_case, err = xparser.GetCase(p.api_file, p.case_name); err != nil {
		return
	}

	if err = parser_instance.BuildRequest(proto, api_case); err != nil {
		return
	}
	err = proto.Flush()

	if skip_result {
		err = SkipResult(proto)
	}

	return
}

func (p *Processor) initMessage() (err error) {
	membuffer := NewTMemoryBuffer()
	membuffer.Open()
	defer membuffer.Close()
	if err = p.flushToTrans(membuffer, false); err != nil {
		return
	}
	p.message = membuffer.GetBytes()
	return
}

func (p *Processor) testCall() (err error) {
	trans := p.tf.GetTransport()
	if err = trans.Open(); err != nil {
		return
	}
	defer trans.Close()

	if p.message == nil {
		err = p.flushToTrans(trans, true)
	} else {
		err = p.writeMessage(trans)
	}
	return
}

func (p *Processor) writeMessage(trans Transport) (err error) {
	if _, err = trans.Write(p.message); err != nil {
		err = fmt.Errorf("can't write byte to trans!")
		return
	}
	proto := p.pf.GetProtocol(p.tw.GetTransport(trans))
	if p.service != "" {
		proto = NewMultiplexedProtocol(proto, p.service)
	}
	err = SkipResult(proto)
	return
}

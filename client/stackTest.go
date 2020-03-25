package client

import (
	"bytes"
	"encoding/binary"
	"log"
	"math/rand"
	"net"
	"os"
	"stackServer/common"
	"sync"
	"testing"
	"time"
)

const (
	connProtocol = "tcp4"
	connHost     = "localhost"
	connPort     = "8081"
	stateBusy    = 0xFF
)

var wg sync.WaitGroup

type stackTest struct {
	conn net.Conn
	t    *testing.T
}

func (st *stackTest) sockConnInit() {
	address := connHost + ":" + connPort
	conn, err := net.Dial(connProtocol, address)
	if err != nil {
		log.Fatalln(err)
		os.Exit(1)
	}
	//log.Printf("Connection success @%s", address)
	st.conn = conn
}

func (st *stackTest) sockConnClose() {
	if st.conn != nil {
		st.conn.Close()
	}
}

func (st *stackTest) push(payload string) int {
	pushStatus := -1
	if payload == "" {
		return pushStatus
	}
	st.sockConnInit()
	defer st.sockConnClose()

	// Handling to send Push request
	payloadLen := len((payload))
	pushReqData := common.GetPushReqData(payload)

	pushRequest := make([]byte, payloadLen+1)
	buf := new(bytes.Buffer)
	buf.WriteByte(pushReqData.Header)
	binary.Read(buf, binary.LittleEndian, pushRequest[0:1])
	buf = bytes.NewBuffer(pushReqData.Payload)
	binary.Read(buf, binary.LittleEndian, pushRequest[1:payloadLen+1])
	n, err := st.conn.Write(pushRequest)
	if (err != nil) || (n < payloadLen) {
		log.Printf("Failed to push pyaload %s due to %+v\n", pushRequest[1:], err)
	} else {
		// Handling to receive Push response
		pushRes := make([]byte, 1)
		_, err := st.conn.Read(pushRes)
		if err != nil {
			log.Printf("Failed to receive Push response due to %s", err)
		} else {
			pushStatus = int(pushRes[0])
			if pushStatus == stateBusy {
				log.Printf("Push for %s was rejected\n\r", pushRequest[1:])
			} else if pushStatus == 0 {
				log.Printf("Pushed '%s'", pushRequest[1:])
			}
		}
	}
	return pushStatus
}

func (st *stackTest) pop() string {
	var payload string

	st.sockConnInit()
	defer st.sockConnClose()

	// Handling to send Pop request
	popReq := make([]byte, 1)
	popReqData := common.GetPopReqData()
	buf := new(bytes.Buffer)
	buf.WriteByte(popReqData.Header)
	binary.Read(buf, binary.LittleEndian, popReq[0:1])
	st.conn.Write(popReq)

	// Handling to receive Pop response
	popRes := make([]byte, 1)
	_, err := st.conn.Read(popRes)
	if err != nil {
		log.Printf("Failed to receive Pop response header due to %s", err)
	} else {
		if popRes[0] == stateBusy {
			log.Println("Pop request was rejected")
		} else {
			payloadLen := int(popRes[0])
			popRes = make([]byte, payloadLen)
			n, err := st.conn.Read(popRes)
			if err != nil || n < payloadLen {
				log.Printf("Failed to receive Pop response data due to %s", err)
			} else {
				payload = string(popRes)
				log.Printf("Poped '%s'", payload)
			}
		}
	}
	return payload
}

// issues one push and one pop
func (st *stackTest) testSigleRequest() {
	log.Println("Running testSigleRequest...")
	payloadLen := common.RandomInt(1, common.MaxPayloadSize)
	payload := common.RandomString(payloadLen)
	st.push(payload)
	popedStr := st.pop()
	common.AssertEqual(st.t, payload, popedStr)
}

// issues N pushes, then N pops.
func (st *stackTest) testSerializedRequests(nTimes int) {
	log.Printf("Running push and pop %d times", nTimes)
	for i := 0; i < nTimes; i++ {
		log.Printf("Running push and pop at %d time", i+1)
		randTimes := common.RandomInt(1, 10)
		expects := make([]string, randTimes)

		log.Printf("Running push %d random times", randTimes)
		for j := 0; j < randTimes; j++ {
			payloadLen := common.RandomInt(1, common.MaxPayloadSize)
			expects[j] = common.RandomString(payloadLen)
			pushStatus := st.push(expects[j])
			common.AssertEqual(st.t, 0, pushStatus)
		}

		log.Printf("Running pop %d random times", randTimes)
		for j := randTimes - 1; j >= 0; j-- {
			popedStr := st.pop()
			common.AssertEqual(st.t, expects[j], popedStr)
		}
	}
}

// RunTests ...
func RunTests() {
	stakTest := new(stackTest)
	rand.Seed(time.Now().UnixNano())

	stakTest.testSigleRequest()
	stakTest.testSerializedRequests(10)
}

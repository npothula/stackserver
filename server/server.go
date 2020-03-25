package server

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"math"
	"net"
	"os"
	"stackServer/common"
	"stackServer/dscollections"
	"sync"
	"time"
)

const (
	connProtocol               = "tcp4" // supported IPv4 only at this time
	connHost                   = "localhost"
	connPort                   = "8080"
	stateBusy                  = 0xFF // Server rejects with stateBusy if connections > 100
	maxStackSize               = 100  // specified for entries count
	durationToRemoveOldestConn = 10   // specified in seconds
	sleepDuration              = 100  // specified in milliseconds
)

type connInfo struct {
	conn          net.Conn
	connTimestamp int64
	payloadSize   uint8
}

var (
	wgServerRun   sync.WaitGroup
	wgProcessReqs sync.WaitGroup

	connReqQueue          *dscollections.Queue
	rwMutexConnQueueCheck sync.RWMutex
	condConnQueueEmpty    *sync.Cond

	pushPendConnReqList     *dscollections.Deque
	rwMutexPushReqListCheck sync.RWMutex
	condPushListEmpty       *sync.Cond

	popPendConnReqList     *dscollections.Deque
	rwMutexPopReqListCheck sync.RWMutex
	condPopListEmpty       *sync.Cond

	stack             *dscollections.Stack
	rwMutexStackCheck sync.RWMutex
	condStackFull     *sync.Cond
	condStackEmpty    *sync.Cond
)

func processPushRequest(connReq *connInfo) {
	log.Printf("Serving Push for %s", connReq.conn.RemoteAddr().String())
	defer connReq.conn.Close()

	tempBuf := make([]byte, connReq.payloadSize)
	payloadBuf := make([]byte, common.MaxPayloadSize)
	var bytesRead uint8 = 0

	//timeoutDuration := 2 * time.Second
	// Reading data from socket
	for bytesRead < connReq.payloadSize {
		//connReq.conn.SetReadDeadline(time.Now().Add(timeoutDuration))
		nbytes, err := connReq.conn.Read(tempBuf)
		if err != nil {
			log.Printf("processPushRequest: read error: %s", err)
			return
		}

		payloadBuf = append(payloadBuf[:bytesRead], tempBuf[:nbytes]...)
		bytesRead += uint8(nbytes)
	}

	// Pushing payload into stack
	condStackFull.L.Lock()
	for stack.Deque.Full() {
		log.Println("stack full, waiting for pop")
		condStackFull.Wait()
	}
	stack.Push(payloadBuf[:bytesRead])
	condStackEmpty.Broadcast()
	condStackFull.L.Unlock()
	log.Printf("Pushed payload: %s", string(payloadBuf[:bytesRead]))

	// Handling to send Push response
	pushResponse := make([]byte, 1)
	buf := new(bytes.Buffer)
	buf.WriteByte(common.GetPushResData().Data)
	binary.Read(buf, binary.LittleEndian, pushResponse[:1])
	connReq.conn.Write(pushResponse)
}

func processPushConnReqests() {
	defer wgProcessReqs.Done()

	for {
		condPushListEmpty.L.Lock()
		for pushPendConnReqList.Size() == 0 {
			log.Println("pushPendConnReqList empty, waiting for push")
			condPushListEmpty.Wait()
		}
		pendConnReq := pushPendConnReqList.Pop()
		condPushListEmpty.L.Unlock()
		connReq := pendConnReq.(*connInfo)
		go processPushRequest(connReq)
	}
}

func processPopRequest(connReq *connInfo) {
	log.Printf("Serving pop for %s", connReq.conn.RemoteAddr().String())
	defer connReq.conn.Close()
	var payload []byte

	//Popping payload from stack
	condStackEmpty.L.Lock()
	for stack.Deque.Empty() {
		log.Println("stack empty, waiting for push")
		condStackEmpty.Wait()
	}
	payload = stack.Pop().([]byte)
	condStackFull.Broadcast()
	condStackEmpty.L.Unlock()

	// Handling to send Pop response
	popResData := common.GetPopResData(payload)
	payloadLen := len(payload)
	popResponse := make([]byte, payloadLen+1)
	buf := new(bytes.Buffer)
	buf.WriteByte(popResData.Header)
	binary.Read(buf, binary.LittleEndian, popResponse[0:1])
	buf = bytes.NewBuffer(popResData.Payload)
	binary.Read(buf, binary.LittleEndian, popResponse[1:payloadLen+1])
	log.Printf("Poped payload: %s", string(popResponse[1:]))
	connReq.conn.Write(popResponse)
}

func processPopConnReqests() {
	defer wgProcessReqs.Done()

	for {
		condPopListEmpty.L.Lock()
		for popPendConnReqList.Size() == 0 {
			log.Println("popPendConnReqList empty, waiting for push")
			condPopListEmpty.Wait()
		}

		pendConnReq := popPendConnReqList.Pop()
		condPopListEmpty.L.Unlock()
		if pendConnReq != nil {
			connReq := pendConnReq.(*connInfo)
			go processPopRequest(connReq)
		}
	}
}

func streamConnRequests() {
	defer wgServerRun.Done()

	// listen for incoming connections
	ln, err := net.Listen(connProtocol, connHost+":"+connPort)
	if err != nil {
		log.Fatalf("Failed to listen due to %s", err.Error())
		os.Exit(1)
	}
	// Close the listener when the application closes.
	defer ln.Close()

	log.Println("Listening on " + connHost + ":" + connPort)
	for {
		// Listen for an incoming connection.
		connReq, err := ln.Accept()
		if err != nil {
			log.Fatalf("Failed to accept connection: %s", err.Error())
			continue
		}
		connInfo := &connInfo{
			conn:          connReq,
			connTimestamp: time.Now().Unix(),
		}
		condConnQueueEmpty.L.Lock()
		connReqQueue.Enqueue(connInfo)
		condConnQueueEmpty.Signal()
		condConnQueueEmpty.L.Unlock()
	}
}

func evaluateConnReqToProcess() {
	defer wgServerRun.Done()

	for {
		condConnQueueEmpty.L.Lock()
		for connReqQueue.Size() == 0 {
			log.Println("connReqQueue empty, waiting for push")
			condConnQueueEmpty.Wait()
		}

		pendConnReq := connReqQueue.Dequeue()
		condConnQueueEmpty.L.Unlock()
		connReq := pendConnReq.(*connInfo)
		pendConnReqCount := pushPendConnReqList.Size() + popPendConnReqList.Size()
		if pendConnReqCount > maxStackSize {
			log.Println(`Connections requests reached max. stack size.
			 Either connection would be rejected or oldest connection would be removed`)

			oldPushPendConnReq := pushPendConnReqList.Last().(connInfo)
			oldPopPendConnReq := popPendConnReqList.Last().(connInfo)
			pushTSDiff := oldPushPendConnReq.connTimestamp - connReq.connTimestamp
			popTSDiff := oldPopPendConnReq.connTimestamp - connReq.connTimestamp
			highTSDiff := int64(math.Max(float64(pushTSDiff), float64(popTSDiff)))
			if uint(highTSDiff) > durationToRemoveOldestConn {
				//Remove oldest connection from corresponding PendConnReqList
				if highTSDiff == pushTSDiff {
					pushPendConnReqList.Pop()
				} else {
					popPendConnReqList.Pop()
				}
				log.Println("Removed oldest connection")
			} else {
				// Reject connection request by sending stateBusy response
				log.Println("Rejecting connection...")
				rejectResponse := make([]byte, 1)
				buf := new(bytes.Buffer)
				buf.WriteByte(stateBusy)
				binary.Read(buf, binary.LittleEndian, rejectResponse)
				connReq.conn.Write(rejectResponse)
				connReq.conn.Close()
				continue
			}
		}

		//Read first byte from accepted connReq for header info
		header := make([]byte, 1)
		_, err := connReq.conn.Read(header)
		if err != nil {
			if err != io.EOF {
				log.Fatalf("evaluateConnReqToProcess: read error : %s", err)
			}
			connReq.conn.Close()
		} else {
			opFlag := header[0] & 0x80 //Check MSB bit to identify for push/pop request
			if opFlag == 0 {
				// Push request
				if header[0] > 0 {
					connReq.payloadSize = header[0]
					if connReq.payloadSize > common.MaxPayloadSize {
						log.Fatalln("Invalid Payload size, it is >maxPayload!!")
						connReq.conn.Close()
					}
					condPushListEmpty.L.Lock()
					pushPendConnReqList.Prepend(connReq)
					condPushListEmpty.Signal()
					condPushListEmpty.L.Unlock()
				} else if header[0] <= 0 {
					log.Fatalln("Received empty header, closing connection.")
					connReq.conn.Close()
				}
			} else {
				// Pop request
				condPopListEmpty.L.Lock()
				popPendConnReqList.Prepend(connReq)
				condPopListEmpty.Signal()
				condPopListEmpty.L.Unlock()

			}
		}
	}
}

func processConnRequests() {
	defer wgServerRun.Done()

	wgProcessReqs.Add(2)
	// Handle push and pop connection requests in separate goroutine.
	go processPushConnReqests()
	go processPopConnReqests()

	wgProcessReqs.Wait()
}

// Run ...
func Run() {
	connReqQueue = dscollections.NewQueue()
	stack = dscollections.NewCappedStack(maxStackSize)
	pushPendConnReqList = dscollections.NewCappedDeque(maxStackSize)
	popPendConnReqList = dscollections.NewCappedDeque(maxStackSize)

	rwMutexStackCheck = sync.RWMutex{}
	condStackFull = sync.NewCond(&rwMutexStackCheck)
	condStackEmpty = sync.NewCond(&rwMutexStackCheck)

	rwMutexConnQueueCheck = sync.RWMutex{}
	condConnQueueEmpty = sync.NewCond(&rwMutexStackCheck)

	rwMutexPushReqListCheck = sync.RWMutex{}
	condPushListEmpty = sync.NewCond(&rwMutexStackCheck)

	rwMutexPopReqListCheck = sync.RWMutex{}
	condPopListEmpty = sync.NewCond(&rwMutexStackCheck)

	wgServerRun.Add(3)

	go streamConnRequests()
	go evaluateConnReqToProcess()
	go processConnRequests()

	wgServerRun.Wait()
}

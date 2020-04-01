package server

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"stackServer/common"
	"stackServer/dscollections"
	"sync"
	"sync/atomic"
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
	maxConnReq                 = 100
)

const (
	connRejected = iota
	connOldRemoved
)

// StackServer ...
type StackServer struct {
	wgServerRun   sync.WaitGroup
	wgProcessReqs sync.WaitGroup

	minHeap         *dscollections.MinHeap
	connInfoMap     map[int64]*common.ConnRequest
	rwMutexConnInfo sync.RWMutex

	stack             *dscollections.Stack
	rwMutexStackCheck sync.RWMutex
	condStackFull     *sync.Cond
	condStackEmpty    *sync.Cond

	connID    int64
	connCount int32
}

func (ss *StackServer) processPushRequest(connInfo *common.ConnInfo) {
	log.Printf("Serving Push for %s", connInfo.ConnReq.Conn.RemoteAddr().String())
	defer func() {
		ss.rwMutexConnInfo.Lock()
		delete(ss.connInfoMap, connInfo.ConnID)
		index := ss.minHeap.Search(connInfo.ConnID, 0)
		if index >= 0 {
			ss.minHeap.Remove(index)
		}
		ss.rwMutexConnInfo.Unlock()

		connInfo.ConnReq.Conn.Close()
		atomic.AddInt32(&ss.connCount, ^int32(0))
		log.Println("closed push request")
	}()

	tempBuf := make([]byte, connInfo.ConnReq.PayloadSize)
	payloadBuf := make([]byte, common.MaxPayloadSize)
	var bytesRead uint8 = 0

	timeoutDuration := 5 * time.Second
	// Reading data from socket
	for bytesRead < connInfo.ConnReq.PayloadSize {
		connInfo.ConnReq.Conn.SetReadDeadline(time.Now().Add(timeoutDuration))
		ss.rwMutexConnInfo.Lock()
		ss.connInfoMap[connInfo.ConnID].ConnStateType = common.ConnRecvState
		ss.rwMutexConnInfo.Unlock()
		nbytes, err := connInfo.ConnReq.Conn.Read(tempBuf)
		if err != nil {
			switch err {
			case io.EOF:
			default:
				log.Printf("processPushRequest: read error: %s", err)
			}
			return
		}

		payloadBuf = append(payloadBuf[:bytesRead], tempBuf[:nbytes]...)
		bytesRead += uint8(nbytes)
	}

	// Pushing payload into stack
	ss.condStackFull.L.Lock()
	for ss.stack.Deque.Full() {
		log.Println("stack full, waiting for pop")
		ss.rwMutexConnInfo.Lock()
		ss.connInfoMap[connInfo.ConnID].ConnStateType = common.ConnWaitState
		ss.rwMutexConnInfo.Unlock()
		ss.condStackFull.Wait()
		duration := time.Now().Unix() - connInfo.ConnReq.ConnTimestamp
		log.Printf("duration: %d, durationToRemoveOldestConn: %d\n", duration, durationToRemoveOldestConn)
		if duration >= durationToRemoveOldestConn {
			log.Println("PushRequest timeout, closing connection")
			ss.condStackFull.L.Unlock()
			return
		}
	}
	ss.stack.Push(payloadBuf[:bytesRead])
	ss.condStackEmpty.Broadcast()
	ss.condStackFull.L.Unlock()
	log.Printf("Pushed payload: %s", string(payloadBuf[:bytesRead]))

	// Handling to send Push response
	pushResponse := make([]byte, 1)
	buf := new(bytes.Buffer)
	buf.WriteByte(common.GetPushResData().Data)
	binary.Read(buf, binary.LittleEndian, pushResponse[:1])
	connInfo.ConnReq.Conn.Write(pushResponse)
}

func (ss *StackServer) processPushConnReqests(pushConnReqChanel <-chan *common.ConnInfo) {
	defer ss.wgProcessReqs.Done()

	for {
		connInfo := <-pushConnReqChanel
		go ss.processPushRequest(connInfo)
		time.Sleep(time.Millisecond * 100)
	}
}

func (ss *StackServer) processPopRequest(connInfo *common.ConnInfo) {
	log.Printf("Serving pop for %s", connInfo.ConnReq.Conn.RemoteAddr().String())
	defer func() {
		ss.rwMutexConnInfo.Lock()
		delete(ss.connInfoMap, connInfo.ConnID)
		index := ss.minHeap.Search(connInfo.ConnID, 0)
		if index >= 0 {
			ss.minHeap.Remove(index)
		}
		ss.rwMutexConnInfo.Unlock()

		connInfo.ConnReq.Conn.Close()
		atomic.AddInt32(&ss.connCount, ^int32(0))
		log.Println("closed pop request")
	}()

	var payload []byte

	//Popping payload from stack
	ss.condStackEmpty.L.Lock()
	for ss.stack.Deque.Empty() {
		log.Println("stack empty, waiting for push")
		ss.rwMutexConnInfo.Lock()
		ss.connInfoMap[connInfo.ConnID].ConnStateType = common.ConnWaitState
		ss.rwMutexConnInfo.Unlock()
		ss.condStackEmpty.Wait()
		log.Printf("processPopRequest: wokedup: %+v", connInfo.ConnReq.ConnTimestamp)
		duration := time.Now().Unix() - connInfo.ConnReq.ConnTimestamp
		log.Printf("duration: %d, durationToRemoveOldestConn: %d\n", duration, durationToRemoveOldestConn)
		if duration >= durationToRemoveOldestConn {
			log.Println("PopRequest timeout, closing connection")
			ss.condStackEmpty.L.Unlock()
			return
		}
	}
	payload = ss.stack.Pop().([]byte)
	ss.condStackFull.Broadcast()
	ss.condStackEmpty.L.Unlock()

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
	connInfo.ConnReq.Conn.Write(popResponse)
}

func (ss *StackServer) processPopConnReqests(popConnReqChanel <-chan *common.ConnInfo) {
	defer ss.wgProcessReqs.Done()

	for {
		connInfo := <-popConnReqChanel
		go ss.processPopRequest(connInfo)
		time.Sleep(time.Millisecond * 100)
	}
}

func (ss *StackServer) processConnRequests(pushConnReqChanel <-chan *common.ConnInfo,
	popConnReqChanel <-chan *common.ConnInfo) {
	defer ss.wgServerRun.Done()

	ss.wgProcessReqs.Add(2)
	// Handle push and pop connection requests in separate goroutine.
	go ss.processPushConnReqests(pushConnReqChanel)
	go ss.processPopConnReqests(popConnReqChanel)

	ss.wgProcessReqs.Wait()
}

func (ss *StackServer) processOverloadedConnection() uint {
	var connStatus uint
	var oldestConnID int64

	log.Println(`Connections requests reached max stack size.
			 Either connection would be rejected or oldest connection would be removed`)

	var connReq *common.ConnRequest
	ss.rwMutexConnInfo.RLock()
	oldestConnID = ss.minHeap.Top()
	connReq, _ = ss.connInfoMap[oldestConnID]
	ss.rwMutexConnInfo.RUnlock()

	duration := time.Now().Unix() - connReq.ConnTimestamp
	log.Printf("processOverloadedConnection: oldest timestamp: %+v, duration: %d, durationToRemoveOldestConn: %d", connReq.ConnTimestamp, duration, durationToRemoveOldestConn)
	if duration >= durationToRemoveOldestConn {
		if connReq.ConnStateType == common.ConnWaitState {
			if connReq.PayloadSize > 0 {
				ss.condStackEmpty.L.Lock()
				//condStackFull.Broadcast()
				ss.condStackFull.Signal()
				ss.condStackEmpty.L.Unlock()
			} else {
				ss.condStackFull.L.Lock()
				//condStackEmpty.Broadcast()
				ss.condStackEmpty.Signal()
				ss.condStackFull.L.Unlock()
			}
		} else if connReq.ConnStateType == common.ConnRecvState {
			connReq.Conn.Close()
			atomic.AddInt32(&ss.connCount, ^int32(0))
			log.Println("Removed oldest connection")
			connStatus = connOldRemoved
		}
	} else {
		// Reject connection request by sending stateBusy response
		rejectResponse := make([]byte, 1)
		buf := new(bytes.Buffer)
		buf.WriteByte(stateBusy)
		binary.Read(buf, binary.LittleEndian, rejectResponse)
		connReq.Conn.Write(rejectResponse)
		log.Println("Rejected connection")
		connReq.Conn.Close()
		atomic.AddInt32(&ss.connCount, ^int32(0))
		connStatus = connRejected
	}
	return connStatus
}

func (ss *StackServer) prepareConnReqToProcess(connInfo *common.ConnInfo,
	pushConnReqChanel chan<- *common.ConnInfo,
	popConnReqChanel chan<- *common.ConnInfo) {
	//Read first byte from accepted connReq for header info
	header := make([]byte, 1)
	_, err := connInfo.ConnReq.Conn.Read(header)
	if err != nil {
		if err != io.EOF {
			log.Fatalf("prepareConnReqToProcess: read error : %s", err)
		}
		connInfo.ConnReq.Conn.Close()
		atomic.AddInt32(&ss.connCount, ^int32(0))
	} else {
		opFlag := header[0] & 0x80 //Check MSB bit to identify for push/pop request
		if opFlag == 0 {
			// Push request
			if header[0] > 0 {
				connInfo.ConnReq.PayloadSize = header[0]
				if connInfo.ConnReq.PayloadSize > common.MaxPayloadSize {
					log.Fatalln("Invalid Payload size, it is >maxPayload!!")
					connInfo.ConnReq.Conn.Close()
					atomic.AddInt32(&ss.connCount, ^int32(0))
				}
				pushConnReqChanel <- connInfo

			} else if header[0] <= 0 {
				log.Fatalln("Received empty header, closing connection.")
				connInfo.ConnReq.Conn.Close()
				atomic.AddInt32(&ss.connCount, ^int32(0))
				return
			}
		} else {
			// Pop request
			popConnReqChanel <- connInfo
		}
		ss.rwMutexConnInfo.Lock()
		ss.connInfoMap[connInfo.ConnID] = connInfo.ConnReq
		ss.minHeap.Insert(connInfo.ConnID)
		ss.rwMutexConnInfo.Unlock()
	}
}

func (ss *StackServer) evaluateConnReqToProcess(connInfoChanel <-chan *common.ConnInfo,
	pushConnReqChanel chan<- *common.ConnInfo,
	popConnReqChanel chan<- *common.ConnInfo) {
	defer ss.wgServerRun.Done()

	for {
		connInfo := <-connInfoChanel
		if ss.connCount > maxStackSize {
			if connRejected == ss.processOverloadedConnection() {
				time.Sleep(time.Millisecond * 100)
				continue
			}
		}
		ss.prepareConnReqToProcess(connInfo, pushConnReqChanel, popConnReqChanel)
		time.Sleep(time.Millisecond * 100)
	}
}

func (ss *StackServer) streamConnRequests(connReqChanel chan<- *common.ConnInfo) {
	defer ss.wgServerRun.Done()

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

		if ss.connCount == 0 {
			// Intialize/ReIntialize connID
			ss.connID = 1
		} else {
			ss.connID++
		}

		connInfo := common.NewConnInfo(ss.connID, connReq)
		connReqChanel <- connInfo
		atomic.AddInt32(&ss.connCount, 1)
		time.Sleep(time.Millisecond * 100)
	}
}

// Run ...
func (ss *StackServer) Run() {
	connInfoChanel := make(chan *common.ConnInfo, maxConnReq)
	pushConnInfoChanel := make(chan *common.ConnInfo, maxConnReq)
	popConnInfoChanel := make(chan *common.ConnInfo, maxConnReq)

	ss.stack = dscollections.NewCappedStack(maxStackSize)
	ss.rwMutexStackCheck = sync.RWMutex{}
	ss.condStackFull = sync.NewCond(&ss.rwMutexStackCheck)
	ss.condStackEmpty = sync.NewCond(&ss.rwMutexStackCheck)

	ss.minHeap = dscollections.NewMinHeap(maxStackSize)
	ss.connInfoMap = map[int64]*common.ConnRequest{}
	ss.rwMutexConnInfo = sync.RWMutex{}

	ss.wgServerRun.Add(4)
	go ss.streamConnRequests(connInfoChanel)
	go ss.evaluateConnReqToProcess(connInfoChanel, pushConnInfoChanel, popConnInfoChanel)
	go ss.processConnRequests(pushConnInfoChanel, popConnInfoChanel)
	ss.wgServerRun.Wait()
}

package common

import (
	"net"
	"time"
)

// ConnLiveState ...
type ConnLiveState uint8

const (
	// ConnIDleState ...
	ConnIDleState ConnLiveState = iota
	// ConnRecvState ...
	ConnRecvState
	// ConnWaitState ...
	ConnWaitState
)

// ConnRequest ...
type ConnRequest struct {
	Conn          net.Conn
	ConnTimestamp int64
	PayloadSize   uint8
	ConnStateType ConnLiveState
}

// ConnInfo ...
type ConnInfo struct {
	ConnID  int64
	ConnReq *ConnRequest
}

// NewConnInfo ...
func NewConnInfo(connID int64, connReq net.Conn) *ConnInfo {
	ConnRequest := &ConnRequest{
		Conn:          connReq,
		ConnTimestamp: time.Now().Unix(),
		PayloadSize:   0,
		ConnStateType: ConnIDleState,
	}

	connInfo := &ConnInfo{
		ConnID:  connID,
		ConnReq: ConnRequest,
	}
	return connInfo
}

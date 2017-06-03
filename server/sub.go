package server

import (
	"net"
)

//this is simple subprotocol impl
//handle all payload

type subprotocol struct {
	conn net.Conn
}

//Handler ... simple interface for websocket impl
type Handler interface {
	//simple websocket method
	Ping() error
	Pong() error
	Send(msg []byte) error
	Receive() error
}

func newSubProtocol(conn net.Conn) Handler {
	return &subprotocol{
		conn: conn,
	}
}

func (sb *subprotocol) Ping() error {
	return nil
}

func (sb *subprotocol) Pong() error {
	return nil
}

func (sb *subprotocol) Send(msg []byte) error {
	return nil
}

func (sb *subprotocol) Receive() error {
	return nil
}

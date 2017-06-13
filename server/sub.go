package server

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
)

const (
	//this is op code
	OP_C byte = 0x0 //denotes a continuation frame
	OP_T byte = 0x1 //denotes a text frame
	OP_B byte = 0x2 //denotes a binary frame
	OP_S byte = 0x8 //denotes a connection close
	OP_P byte = 0x9 //ping
	OP_G byte = 0xA //pong
	//other for further
)

//MessageHandler ...
type MessageHandler interface {
	HandleTextMessage(msg string, reply Handler) error
	HandleBinaryMessage(msg []byte, reply Handler) error
}

//define header
//FIN,RSV1, RSV2, RSV3
/*
    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
   +-+-+-+-+-------+-+-------------+-------------------------------+
   |F|R|R|R| opcode|M| Payload len |    Extended payload length    |
   |I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
   |N|V|V|V|       |S|             |   (if payload len==126/127)   |
   | |1|2|3|       |K|             |                               |
   +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
   |     Extended payload length continued, if payload len == 127  |
   + - - - - - - - - - - - - - - - +-------------------------------+
   |                               |Masking-key, if MASK set to 1  |
   +-------------------------------+-------------------------------+
   | Masking-key (continued)       |          Payload Data         |
   +-------------------------------- - - - - - - - - - - - - - - - +
   :                     Payload Data continued ...                :
   + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
   |                     Payload Data continued ...                |
   +---------------------------------------------------------------+

*/
//this is simple subprotocol impl
//handle all payload
//simple Frame struct
type frame struct {
	fin         byte //first bit of message
	opcode      byte //to [4]uint
	mask        byte //uint
	payLoadLen  byte //[7]uint
	extLen      uint64
	maskingKey  []byte
	payloadData []byte
}

type defaultHandler struct {
}

func (d *defaultHandler) HandleTextMessage(msg string, reply Handler) error {
	log.Printf("handle text data %s \n", msg)
	return nil
}

func (d *defaultHandler) HandleBinaryMessage(msg []byte, reply Handler) error {
	log.Printf("handle binary data %s \n", msg)
	return nil
}

func createPingMessage(data []byte) []byte {
	if data != nil {
		dl := len(data)
		d := []byte{OP_P, byte(dl)}
		d = append(d, data...)
		return d
	}
	return []byte{OP_P, 0x0}
}

func createPongMessage(data []byte) []byte {
	if data != nil {
		dl := len(data)
		d := []byte{OP_G, byte(dl)}
		d = append(d, data...)
		return d
	}
	return []byte{OP_G, 0x0}
}

type subprotocol struct {
	conn    net.Conn
	handler MessageHandler
}

//Handler ... simple interface for websocket impl
type Handler interface {
	//simple websocket method
	ping(data []byte) error
	pong(data []byte) error
	Send(msg []byte) error
	handleMessage() error
}

func newSubProtocol(conn net.Conn, handler MessageHandler) Handler {
	return &subprotocol{
		conn:    conn,
		handler: handler,
	}
}

func (sb *subprotocol) ping(data []byte) error {
	_, err := sb.conn.Write(createPingMessage(data))
	return err
}

func (sb *subprotocol) pong(data []byte) error {
	_, err := sb.conn.Write(createPongMessage(data))
	return err
}

func (sb *subprotocol) Send(msg []byte) error {
	_, err := sb.conn.Write(msg)
	return err
}

func (sb *subprotocol) handleMessage() error {
	for {
		var buffer = make([]byte, 2)
		n, err := sb.conn.Read(buffer)
		if err != nil {
			//log.Printf("handle error %s \n", err)
			return err
		}
		f := &frame{}
		if n < 2 {
			log.Printf("size is too short %s \n", err)
			continue
		}
		err = sb.validate(buffer[0], f)
		if err != nil {
			sb.conn.Close()
			log.Printf("header is invalid %s \n", err)
			continue
		}
		//mask and len
		sb.maskAndLen(buffer[1], f)
		if f.payLoadLen == 126 {
			var extBuffer = make([]byte, 2)
			n, err := sb.conn.Read(extBuffer)
			if n != 2 || err != nil {
				log.Printf("error when read extestion len %s \n", err)
				sb.conn.Close()
				continue
			}
			f.extLen, _ = binary.Uvarint(extBuffer)
		} else if f.payLoadLen == 127 {
			var extBuffer = make([]byte, 8)
			n, err := sb.conn.Read(extBuffer)
			if n != 8 || err != nil {
				log.Printf("error when read extestion len %s \n", err)
				continue
			}
			f.extLen, _ = binary.Uvarint(extBuffer)
		}
		if f.mask == 0 {
			//TODO
		} else {
			var maskBuffer = make([]byte, 4)
			n, err := sb.conn.Read(maskBuffer)
			if n != 4 || err != nil {
				log.Printf("err mask data %s \n", err)
				continue
			}
			f.maskingKey = maskBuffer
		}
		//read data from connection
		totalLen := binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, f.payLoadLen}) + f.extLen
		f.payloadData = make([]byte, totalLen)
		n, err = sb.conn.Read(f.payloadData)
		if err != nil {
			log.Printf("handle error data %s \n", err)
			continue
		}
		if uint64(n) != totalLen {
			log.Printf("read data error")
			continue
		}
		sb.translate(f.payloadData[:totalLen], f.maskingKey)
		switch f.opcode {
		case OP_P:
			sb.pong(nil)
			break
		case OP_G:
			sb.ping(nil)
			break
		case OP_T:
			sb.handler.HandleTextMessage(string(f.payloadData[:totalLen]), sb)
			break
		case OP_B:
			sb.handler.HandleBinaryMessage(f.payloadData[:totalLen], sb)
			break
		case OP_S:
			sb.close()
			break
		default:
			break
		}
	}
}

func (sb *subprotocol) translate(payload, maskKey []byte) {
	for i := 0; i < len(payload); i++ {
		payload[i] = payload[i] ^ maskKey[i%4]
	}
}

func (sb *subprotocol) close() error {
	return sb.conn.Close()
}

func (sb *subprotocol) maskAndLen(segment byte, f *frame) {
	f.mask = (segment >> 7) & 1
	f.payLoadLen = (segment << 1) >> 1
}

//return opcode and error
func (sb *subprotocol) validate(frameHeader byte, f *frame) error {
	//validate fin
	//current only support simple impl
	f.fin = (frameHeader >> 7) & 1
	v := (frameHeader << 1) >> 5
	log.Printf("value is %b \n", v)
	var i byte = 8
	for ; i > 0; i = i >> 1 {
		if v&i != 0 {
			return fmt.Errorf("error rsv code")
		}
	}
	f.opcode = (frameHeader << 4) >> 4
	return nil
}

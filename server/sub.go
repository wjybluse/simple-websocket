package server

import (
	"encoding/binary"
	"log"
	"net"

	"strings"

	"fmt"

	"github.com/elians/websocket/utils/alg"
)

const (
	opc byte = 0x0 //denotes a continuation frame
	opt byte = 0x1 //denotes a text frame
	opb byte = 0x2 //denotes a binary frame
	ops byte = 0x8 //denotes a connection close
	opp byte = 0x9 //ping
	opg byte = 0xA //pong
	//other for further

	payloadFixLen = 126
	payloadMaxLen = 127

	extLenFix = 16
	extLenMax = 64
)

const (
	normlClose int16 = 1000 + iota
	goAway
	protocolErr
	acceptErr
	reserved
	reserved1
	abnormal
	typeErr
	outsideErr
	msgBigErr
	extensionNotSupport
	requestErr
	tlsErr
)

//MessageHandler ...
type MessageHandler interface {
	HandleTextMessage(msg string, reply Handler) error
	HandleBinMessage(msg []byte, reply Handler) error
	HandleError(err []byte, reply Handler) error
	HandlePing(data []byte, reply Handler) error
	HandlePong(data []byte, reply Handler) error
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
	fin         byte
	opcode      byte
	mask        byte
	payLoadLen  byte
	extLen      uint64
	maskingKey  []byte
	payloadData []byte
	rsv         byte
}

func (f *frame) toBytes() []byte {
	paylen := len(f.payloadData)
	data := []byte{(f.fin << 7) | (f.rsv << 4) | f.opcode}
	if paylen > payloadMaxLen {
		var buffer []byte
		if paylen > (1<<extLenFix)+payloadFixLen {
			f.payLoadLen = payloadMaxLen
			f.extLen = uint64(paylen - payloadMaxLen)
			binary.BigEndian.PutUint64(buffer, f.extLen)

		} else {
			f.payLoadLen = payloadFixLen
			f.extLen = uint64(paylen - payloadFixLen)
			binary.BigEndian.PutUint16(buffer, uint16(f.extLen))
		}
		data = append(data, (f.mask<<7)|f.payLoadLen)
		data = append(data, buffer...)

	} else {
		f.payLoadLen = byte(paylen)
		data = append(data, (f.mask<<7)|f.payLoadLen)
	}
	if f.mask == 1 {
		data = append(data, f.maskingKey[:4]...)
	}
	data = append(data, f.payloadData...)
	return data
}

func (f *frame) String() string {
	return fmt.Sprintf(`
		fin:%v,
		opcode:%v,
		mask: %v,
		paylen: %v,
		extlen: %d,
		maskkey: %v,
		payload: %v,
		rsv: %v
	`, f.fin, f.opcode, f.mask, f.payLoadLen, f.extLen, f.maskingKey, f.payloadData, f.rsv)
}

func newFrame(fin, opcode, mask, rsv byte, maskingKey, payload []byte) *frame {
	return &frame{
		fin:         fin,
		opcode:      opcode,
		mask:        mask,
		rsv:         rsv,
		maskingKey:  maskingKey,
		payloadData: payload,
	}
}

type dhanler struct {
}

func (d *dhanler) HandleTextMessage(msg string, reply Handler) error {
	log.Printf("handle text data %s \n", msg)
	return reply.Send([]byte("nihao"))
}

func (d *dhanler) HandleBinMessage(msg []byte, reply Handler) error {
	log.Printf("handle binary data %s \n", msg)
	return nil
}

func (d *dhanler) HandleError(err []byte, reply Handler) error {
	return nil
}

func (d *dhanler) HandlePing(data []byte, reply Handler) error {
	return nil
}

func (d *dhanler) HandlePong(data []byte, reply Handler) error {
	return nil
}

type subConn struct {
	conn       net.Conn
	handler    MessageHandler
	frame      *frame
	extensions []string
}

//Handler ... simple interface for websocket impl
type Handler interface {
	//simple websocket method
	ping(data []byte) error
	pong(data []byte) error
	Send(msg []byte) error
	handleMessage() error
}

//NewHandler handler sub protocol
func NewHandler(conn net.Conn, handler MessageHandler, extensions []string) Handler {
	return &subConn{
		conn:       conn,
		handler:    handler,
		extensions: extensions,
	}
}

func (sc *subConn) enableDeflat() bool {
	for _, e := range sc.extensions {
		if strings.Contains(e, "deflat") {
			return true
		}
	}
	return false
}

func (sc *subConn) ping(data []byte) error {
	log.Printf("send ping frame,pong message is %s \n", string(data))
	f := newFrame(0, opp, 0, 0, nil, []byte("are u ok?"))
	_, err := sc.conn.Write(f.toBytes())
	return err
}

func (sc *subConn) pong(data []byte) error {
	log.Printf("send pong frame,the ping message is %s \n", string(data))
	f := newFrame(0, opg, 0, 0, nil, []byte("i'm mibody"))
	_, err := sc.conn.Write(f.toBytes())
	return err
}

func (sc *subConn) Send(msg []byte) error {
	content := msg
	var err error
	if sc.enableDeflat() {
		deflat := alg.Deflat{}
		content, err = deflat.Encoding(msg)
		if err != nil {
			log.Printf("encoding data failed")
			return err
		}
	}
	f := newFrame(sc.frame.fin, sc.frame.opcode, sc.frame.mask, sc.frame.rsv, sc.frame.maskingKey, content)
	if f.mask == 1 {
		sc.translate(f.payloadData, f.maskingKey)
	}
	_, err = sc.conn.Write(f.toBytes())
	return err
}

func (sc *subConn) createFrame() (*frame, error) {
	var buffer = make([]byte, 2)
	n, err := sc.conn.Read(buffer)
	if err != nil {
		//log.Printf("handle error %s \n", err)
		return nil, err
	}
	f := &frame{}
	if n < 2 {
		log.Printf("size is too short %s \n", err)
		return nil, fmt.Errorf("error size")
	}
	err = sc.setOpcode(buffer[0], f)
	if err != nil {
		log.Printf("header is invalid %s \n", err)
		return nil, err
	}
	//mask and len
	sc.setMaskAndLen(buffer[1], f)
	if f.payLoadLen == payloadFixLen {
		var extBuffer = make([]byte, 2)
		n, err := sc.conn.Read(extBuffer)
		if n != 2 || err != nil {
			log.Printf("error when read extestion len %s \n", err)
			return nil, err
		}
		f.extLen, _ = binary.Uvarint(extBuffer)
	} else if f.payLoadLen == payloadMaxLen {
		var extBuffer = make([]byte, 8)
		n, err := sc.conn.Read(extBuffer)
		if n != 8 || err != nil {
			log.Printf("error when read extestion len %s \n", err)
			return nil, fmt.Errorf("invalid size or error")
		}
		f.extLen, _ = binary.Uvarint(extBuffer)
	}
	if f.mask == 0 {
		//TODO
	} else {
		var maskBuffer = make([]byte, 4)
		n, err := sc.conn.Read(maskBuffer)
		if n != 4 || err != nil {
			log.Printf("err mask data %s \n", err)
			return nil, err
		}
		f.maskingKey = maskBuffer
	}
	//read data from connection
	totalLen := binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, f.payLoadLen}) + f.extLen
	f.payloadData = make([]byte, totalLen)
	n, err = sc.conn.Read(f.payloadData)
	if err != nil {
		log.Printf("handle error data %s \n", err)
		return nil, err
	}
	if uint64(n) != totalLen {
		log.Printf("read data error")
		return nil, err
	}
	sc.translate(f.payloadData[:totalLen], f.maskingKey)
	if sc.enableDeflat() {
		f.payloadData, _ = sc.decode(f.payloadData[:totalLen])
	}
	if err != nil {
		log.Printf("decode failed %s \n", err)
		sc.conn.Close()
		return nil, err
	}
	return f, nil
}

func (sc *subConn) handleMessage() error {
	for {
		f, err := sc.createFrame()
		if err != nil {
			continue
		}
		sc.frame = f
		switch f.opcode {
		case opp:
			sc.pong(f.payloadData)
			break
		case opg:
			sc.ping(f.payloadData)
			break
		case opt:
			sc.handler.HandleTextMessage(string(f.payloadData), sc)
			break
		case opb:
			sc.handler.HandleBinMessage(f.payloadData, sc)
			break
		case ops:
			sc.close(normlClose)
			break
		default:
			sc.close(acceptErr)
			break
		}
	}
}
func (sc *subConn) decode(payload []byte) ([]byte, error) {
	deflat := alg.Deflat{}
	return deflat.Decoding(payload)
}
func (sc *subConn) translate(payload, maskKey []byte) {
	for i := 0; i < len(payload); i++ {
		payload[i] = payload[i] ^ maskKey[i%4]
	}
}

func (sc *subConn) close(status int16) error {
	log.Printf("close connection....")
	f := newFrame(0x0, ops, 0x0, 0x0, nil, []byte{byte(status >> 8), byte(status)})
	_, err := sc.conn.Write(f.toBytes())
	if err != nil {
		log.Printf("send data error %s \n", err)
	}
	return sc.conn.Close()
}

func (sc *subConn) setMaskAndLen(segment byte, f *frame) {
	//shift right
	f.mask = (segment & 0x80) >> 7
	f.payLoadLen = segment & 0x7f
}

//return opcode and error
func (sc *subConn) setOpcode(frameHeader byte, f *frame) error {
	f.fin = (frameHeader & 0x80) >> 7
	f.rsv = (frameHeader & 0x70) >> 4
	f.opcode = frameHeader & 0x0f
	return nil
}

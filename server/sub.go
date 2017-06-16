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

	payloadFixLen = 0x7f - 0x01
	payloadMaxLen = 0x7f

	extLenFix = 0x10
	extLenMax = 0x40
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
	fin     byte
	opcode  byte
	mask    byte
	pl      byte
	el      uint64
	mkey    []byte
	payload []byte
	rsv     byte
}

func (f *frame) toBytes() []byte {
	paylen := len(f.payload)
	data := []byte{(f.fin << 0x07) | (f.rsv << 0x04) | f.opcode}
	if paylen > payloadMaxLen {
		var buffer []byte
		if paylen > (1<<extLenFix)+payloadFixLen {
			f.pl = payloadMaxLen
			f.el = uint64(paylen - payloadMaxLen)
			binary.BigEndian.PutUint64(buffer, f.el)

		} else {
			f.pl = payloadFixLen
			f.el = uint64(paylen - payloadFixLen)
			binary.BigEndian.PutUint16(buffer, uint16(f.el))
		}
		data = append(data, (f.mask<<0x07)|f.pl)
		data = append(data, buffer...)

	} else {
		f.pl = byte(paylen)
		data = append(data, (f.mask<<7)|f.pl)
	}
	if f.mask == 1 {
		data = append(data, f.mkey[:4]...)
	}
	data = append(data, f.payload...)
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
	`, f.fin, f.opcode, f.mask, f.pl, f.el, f.mkey, f.payload, f.rsv)
}

func newFrame(fin, opcode, mask, rsv byte, mkey, payload []byte) *frame {
	return &frame{
		fin:     fin,
		opcode:  opcode,
		mask:    mask,
		rsv:     rsv,
		mkey:    mkey,
		payload: payload,
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
	return reply.Send(alg.RandomMask())
}

func (d *dhanler) HandleError(err []byte, reply Handler) error {
	log.Printf("handle error message %s \n", string(err))
	return nil
}

func (d *dhanler) HandlePing(data []byte, reply Handler) error {
	log.Printf("handle ping message %s \n", string(data))
	return reply.Pong([]byte("i'm mibody!"))
}

func (d *dhanler) HandlePong(data []byte, reply Handler) error {
	log.Printf("handle pong message %s \n", string(data))
	return reply.Ping([]byte("are u ok?"))
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
	Ping(data []byte) error
	Pong(data []byte) error
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
		if strings.Contains(e, "permessage-deflate") {
			return true
		}
	}
	return false
}

func (sc *subConn) Ping(data []byte) error {
	f := newFrame(0, opp, 0, 0, nil, data)
	_, err := sc.conn.Write(f.toBytes())
	return err
}

func (sc *subConn) Pong(data []byte) error {
	f := newFrame(0, opg, 0, 0, nil, data)
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
	//mask not allowed in server side???
	// maskkey := alg.RandomMask()
	// if sc.frame.mask == 0 {
	// 	maskkey = nil
	// }
	f := newFrame(sc.frame.fin, sc.frame.opcode, 0x0, sc.frame.rsv, nil, content)
	// if f.mask == 1 {
	// 	translate(f.payload, f.mkey)
	// }
	_, err = sc.conn.Write(f.toBytes())
	return err
}

func (sc *subConn) createFrame() (*frame, error) {
	var buffer = make([]byte, 2)
	n, err := sc.conn.Read(buffer)
	if err != nil {
		return nil, err
	}
	f := &frame{}
	if n < 2 {
		log.Printf("size is too short %s \n", err)
		sc.conn.Write(closeFrame(abnormal).toBytes())
		return nil, fmt.Errorf("error size")
	}
	setOpcode(buffer[0], f)
	//mask and len
	setMaskAndLen(buffer[1], f)
	if f.pl == payloadFixLen {
		var extBuffer = make([]byte, 2)
		n, err := sc.conn.Read(extBuffer)
		if n != 2 || err != nil {
			sc.conn.Write(closeFrame(abnormal).toBytes())
			return nil, err
		}
		f.el, _ = binary.Uvarint(extBuffer)
	} else if f.pl == payloadMaxLen {
		var extBuffer = make([]byte, 8)
		n, err := sc.conn.Read(extBuffer)
		if n != 8 || err != nil {
			log.Printf("error when read extestion len %s \n", err)
			sc.conn.Write(closeFrame(abnormal).toBytes())
			return nil, fmt.Errorf("invalid size or error")
		}
		f.el, _ = binary.Uvarint(extBuffer)
	}
	if f.mask != 0 {
		var mkey = make([]byte, 4)
		n, err := sc.conn.Read(mkey)
		if n != 4 || err != nil {
			log.Printf("err mask data %s \n", err)
			return nil, err
		}
		f.mkey = mkey
	}
	//read data from connection
	totalLen := binary.BigEndian.Uint64([]byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, f.pl}) + f.el
	f.payload = make([]byte, totalLen)
	n, err = sc.conn.Read(f.payload)
	if err != nil {
		log.Printf("handle error data %s \n", err)
		return nil, err
	}
	if uint64(n) != totalLen {
		log.Printf("read data error")
		return nil, err
	}
	translate(f.payload[:totalLen], f.mkey)
	if sc.enableDeflat() {
		f.payload, _ = sc.decode(f.payload[:totalLen])
	}
	if err != nil {
		log.Printf("decode failed %s \n", err)
		sc.conn.Write(closeFrame(extensionNotSupport).toBytes())
		return nil, err
	}
	return f, nil
}

func (sc *subConn) handleMessage() error {
	for {
		f, err := sc.createFrame()
		if err != nil {
			sc.conn.Close()
			continue
		}
		sc.frame = f
		switch f.opcode {
		case opp:
			sc.handler.HandlePing(f.payload, sc)
			break
		case opg:
			sc.handler.HandlePong(f.payload, sc)
			break
		case opt:
			sc.handler.HandleTextMessage(string(f.payload), sc)
			break
		case opb:
			sc.handler.HandleBinMessage(f.payload, sc)
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
func translate(payload, maskKey []byte) {
	for i := 0; i < len(payload); i++ {
		payload[i] = payload[i] ^ maskKey[i%4]
	}
}

func (sc *subConn) close(closeCode int16) error {
	// bigEndian or littleEndian?
	_, err := sc.conn.Write(closeFrame(closeCode).toBytes())
	if err != nil {
		log.Printf("send close frame error %s \n", err)
	}
	return sc.conn.Close()
}

func closeFrame(code int16) *frame {
	return newFrame(0x0, ops, 0x0, 0x0, nil, []byte{byte(code >> 0x08), byte(code)})
}

func setMaskAndLen(segment byte, f *frame) {
	f.mask = (segment & 0x80) >> 0x07
	f.pl = segment & 0x7f
}

//set opcode ,fin and rsv
func setOpcode(segment byte, f *frame) {
	//one byte as one segment
	f.fin = (segment & 0x80) >> 0x07
	f.rsv = (segment & 0x70) >> 0x04
	f.opcode = segment & 0x0f
}

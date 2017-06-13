package server

import (
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/elians/websocket/utils"
)

const (
	guid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	p    = "chat"
)

type websocket struct {
	host string
	//websocket location
	url string
}
type header struct {
	name, value string
}

//Headers ... new type define
type Headers []*header

func replyHeader() Headers {
	var headers = make([]*header, 0)
	headers = append(headers,
		&header{
			name:  "Upgrade",
			value: "websocket",
		},
		&header{
			name:  "Connection",
			value: "Upgrade",
		},
		&header{
			name:  "status",
			value: "HTTP/1.1 101 Switching Protocols",
		})
	return headers
}

func badReply() Headers {
	var headers = make([]*header, 0)
	headers = append(headers,
		&header{
			name:  "status",
			value: "HTTP/1.1 400 Bad Request",
		},
		&header{
			name:  "Cache-Control",
			value: "no-cache",
		})
	return headers
}

//Get ... get value
func (hs Headers) Get(key string) []string {
	var values = make([]string, 0)
	for _, v := range hs {
		if v.name == key {
			values = append(values, v.value)
		}
	}
	return values
}

//Put ... add one item
func (hs Headers) Put(key, value string) Headers {
	var header = &header{
		name:  key,
		value: value,
	}
	hs = append(hs, header)
	return hs
}

//Headers ... get all headers
func (hs Headers) Headers() map[string][]string {
	var result = make(map[string][]string, 0)
	for _, v := range hs {
		if result[v.name] == nil {
			result[v.name] = make([]string, 0)
		}
		result[v.name] = append(result[v.name], v.value)
	}
	return result
}

//AddHeader ... add header
func (hs Headers) AddHeader(headers map[string]string) Headers {
	for key, value := range headers {
		hs = append(hs, &header{
			name:  key,
			value: value,
		})
	}
	return hs
}

func (hs Headers) String() string {
	r := ""
	method := ""
	path := ""
	p := ""
	status := ""
	for _, v := range hs {
		if v.name == "method" {
			method = v.value
			continue
		}
		if v.name == "status" {
			status = v.value
			continue
		}
		if v.name == "path" {
			path = v.value
			continue
		}
		if v.name == "protocol" {
			p = v.value
			continue
		}
		r += fmt.Sprintf("%s: %s \r\n", v.name, v.value)
	}
	s := fmt.Sprintf("%s %s %s", method, path, p)
	if method == "" || path == "" || p == "" {
		s = status
	}
	idx := strings.LastIndex(r, "\r\n")
	return fmt.Sprintf("%s\r\n%s\r\n\r\n", s, r[:idx])
}

type protocol interface {
	handshake(headers Headers) error
	upgrade(conn net.Conn, headers Headers) error
}

//Listener ...listener interface
type Listener interface {
	Listen() error
}

//NewWebsocket ... return new websocket
func NewWebsocket(host, url string) Listener {
	return &websocket{
		host: host,
		url:  url,
	}
}

func (w *websocket) Listen() error {
	listen, err := net.Listen("tcp", w.host)
	if err != nil {
		return err
	}
	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Printf("error connection handler,%s", err)
			continue
		}
		go w.handleConn(conn)
	}
}

func (w *websocket) handleConn(conn net.Conn) error {
	var buf = make([]byte, 7*1024)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	if n > 7*1024 {
		//TODO
		//read again
	} else {
		buf = buf[:n]
	}
	headers := w.parserHeaders(buf)
	err = w.handshake(headers)
	if err != nil {
		reply := badReply()
		if strings.Contains(err.Error(), "version") {
			reply.Put("Sec-WebSocket-Version", "13, 8, 7")
		}
		defer conn.Close()
		conn.Write([]byte(reply.String()))
		fmt.Printf("handle error is %s \n", err)
		return err
	}
	hs := replyHeader()
	cid := Headers(headers).Get("Sec-WebSocket-Key")[0]
	hs = hs.Put("Sec-WebSocket-Accept", utils.Base64Encoding(utils.Sha1(cid+guid)))
	extensions := Headers(headers).Get("Sec-WebSocket-Extensions")
	if len(extensions) > 0 {
		ext := extensions[0]
		if strings.Contains(ext, "client_max_window_bits") {
			ext = strings.Replace(ext, "client_max_window_bits", "client_max_window_bits=10", -1)
		}
		hs = hs.Put("Sec-WebSocket-Extensions", ext)
	}
	conn.Write([]byte(hs.String()))
	return nil
}

func (w *websocket) parserHeaders(buf []byte) []*header {
	hstr := string(buf)
	var headers = make([]*header, 0)
	lines := strings.Split(hstr, "\r\n")
	//method and host
	mh := strings.Split(lines[0], " ")
	if len(mh) < 3 {
		log.Printf("invalid header %s \n", lines[0])
		return nil
	}
	headers = append(headers, &header{
		name:  "method",
		value: strings.TrimSpace(mh[0]),
	})
	headers = append(headers, &header{
		name:  "path",
		value: strings.TrimSpace(mh[1]),
	})
	headers = append(headers, &header{
		name:  "version",
		value: strings.TrimSpace(mh[2]),
	})
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		pair := strings.Split(line, ":")
		if len(pair) < 2 {
			continue
		}
		headers = append(headers, &header{
			name:  strings.TrimSpace(pair[0]),
			value: strings.TrimSpace(pair[1]),
		})
	}
	return headers
}

func (w *websocket) handshake(headers Headers) error {
	if headers.Get("method")[0] != "GET" {
		//TODO method invalid
		return fmt.Errorf("error method")
	}
	if w.url != headers.Get("path")[0] {
		//TODO
		return fmt.Errorf("invalid path")
	}
	upgrade := headers.Get("Upgrade")
	if upgrade == nil || len(upgrade) <= 0 {
		return fmt.Errorf("upgrade error")
	}
	connections := headers.Get("Connection")
	if len(connections) <= 0 || connections[0] != "Upgrade" {
		return fmt.Errorf("upgrade error")
	}
	if upgrade[0] != "websocket" {
		return fmt.Errorf("upgrade error")
	}
	log.Printf("header is %s \n", headers.String())
	key := headers.Get("Sec-WebSocket-Key")
	//may include
	//pt := headers.Get("Sec-WebSocket-Protocol")
	version := headers.Get("Sec-WebSocket-Version")
	if key == nil || len(key) == 0 || key[0] == "" {
		return fmt.Errorf("key is invalid")
	}
	if version == nil || len(version) <= 0 || version[0] == "" || version[0] != "13" {
		return fmt.Errorf("version is invalid")
	}
	return nil
}

func (w *websocket) upgrade(conn net.Conn, headers Headers) error {
	//TODO
	return nil
}

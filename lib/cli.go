package lib

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
)

type Client struct {
	host string
	port int
	conn *net.TCPConn
}

func NewClient(h string, p int) *Client {
	return &Client{
		host: h,
		port: p,
	}
}

func (c *Client) Connect() error {
	var err error
	tcpAddr := &net.TCPAddr{IP: net.ParseIP(c.host), Port: c.port}
	c.conn, err = net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) IsConnected() bool {
	_, err := c.conn.Write(nil)
	return err != nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) DoRequest(cmd []byte, args ...[]byte) (int, error) {
	return c.conn.Write(Multi(cmd, args...))
}

func (c *Client) GetReply() (*Reply, error) {
	rd := bufio.NewReader(c.conn)
	b, err := rd.Peek(1)
	if err != nil {
		return nil, err
	}

	var reply *Reply
	if b[0] == byte('*') {
		multiBulk, err := MultiUn(rd)
		if err != nil {
			return nil, err
		}
		reply = &Reply{
			multiBulk: multiBulk,
			isMulti:   true,
		}
	} else {
		bulk, err := BulkUnMarshal(rd)
		if err != nil {
			return nil, err
		}
		reply = &Reply{
			bulk:    bulk,
			isMulti: false,
		}
	}

	return reply, nil
}

func (c *Client) Result(str []byte) (*Reply, error) {
	fields := bytes.Fields(str)
	_, err := c.DoRequest(fields[0], fields[1:]...)
	if err != nil {
		return nil, err
	}

	rst, err := c.GetReply()
	if err != nil {
		return nil, err
	}
	return rst, nil
}

type Reply struct {
	isMulti   bool
	bulk      []byte
	multiBulk [][]byte
}

func (r *Reply) Format() []string {
	if !r.isMulti {
		if r.bulk == nil {
			return []string{fmt.Sprint("(nil)")}
		}
		return []string{fmt.Sprint(string(r.bulk))}
	}

	if r.multiBulk == nil {
		return []string{fmt.Sprint("(nil)")}
	}
	if len(r.multiBulk) == 0 {
		return []string{fmt.Sprint("(empty list or set)")}
	}
	out := make([]string, len(r.multiBulk))
	for i := 0; i < len(r.multiBulk); i++ {
		if r.multiBulk[i] == nil {
			out[i] = fmt.Sprintf("%d) (nil)", i)
		} else {
			out[i] = fmt.Sprintf("%d) \"%s\"", i, r.multiBulk[i])
		}
	}
	return out
}

func BulkUnMarshal(rd *bufio.Reader) ([]byte, error) {
	b, err := rd.ReadByte()
	if err != nil {
		return []byte{}, err
	}

	var result []byte
	switch b {
	case byte('+'), byte('-'), byte(':'):
		r, _, err := rd.ReadLine()
		if err != nil {
			return []byte{}, err
		}
		result = r
	case byte('$'):
		r, _, err := rd.ReadLine()
		if err != nil {
			return []byte{}, err
		}

		l, err := strconv.Atoi(string(r))
		if err != nil {
			return []byte{}, err
		}

		if l == -1 {
			return nil, nil
		}

		p := make([]byte, l+2)
		rd.Read(p)
		result = p[0 : len(p)-2]
	}

	return result, nil
}

func MultiUn(rd *bufio.Reader) ([][]byte, error) {
	b, err := rd.ReadByte()
	if err != nil {
		return [][]byte{}, err
	}

	if b != '*' {
		return [][]byte{}, errors.New("client: wrong protocol fromat")
	}

	bNum, _, err := rd.ReadLine()
	if err != nil {
		return [][]byte{}, err
	}
	n, err := strconv.Atoi(string(bNum))
	if err != nil {
		return [][]byte{}, err
	}

	if n == 0 {
		return [][]byte{}, nil
	}
	if n == -1 {
		return nil, nil
	}

	result := make([][]byte, n)
	for i := 0; i < n; i++ {
		result[i], err = BulkUnMarshal(rd)
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

func Multi(cmd []byte, args ...[]byte) []byte {
	buffer := new(bytes.Buffer)
	buffer.WriteByte('*')
	buffer.WriteString(strconv.Itoa(len(args) + 1))
	buffer.Write([]byte{'\r', '\n'})

	buffer.WriteByte('$')
	buffer.WriteString(strconv.Itoa(len(cmd)))
	buffer.Write([]byte{'\r', '\n'})
	buffer.Write(cmd)
	buffer.Write([]byte{'\r', '\n'})

	for _, v := range args {
		buffer.WriteByte('$')
		buffer.WriteString(strconv.Itoa(len(v)))
		buffer.Write([]byte{'\r', '\n'})
		buffer.Write(v)
		buffer.Write([]byte{'\r', '\n'})
	}

	return buffer.Bytes()
}

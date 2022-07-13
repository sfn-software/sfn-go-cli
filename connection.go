package main

import (
	"bufio"
	"net"
)

type Connection struct {
	ln     net.Listener
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

func (c *Connection) Listen(address string) (string, error) {
	var err error
	err = c.StopListen()

	c.ln, err = net.Listen("tcp", address)
	if err != nil || c.ln == nil {
		return "", err
	}

	if c.conn, err = c.ln.Accept(); err != nil {
		return "", err
	}

	c.reader = bufio.NewReader(c.conn)
	c.writer = bufio.NewWriter(c.conn)

	return c.conn.RemoteAddr().String(), nil
}

func (c *Connection) StopListen() error {
	if c.ln != nil {
		err := c.ln.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Connection) Connect(address string) (string, error) {
	var err error
	if c.conn, err = net.Dial("tcp", address); err != nil {
		return "", err
	}

	c.reader = bufio.NewReader(c.conn)
	c.writer = bufio.NewWriter(c.conn)

	return c.conn.RemoteAddr().String(), nil
}

func (c *Connection) Disconnect() error {
	if c.conn != nil {
		err := c.conn.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func NewConnection() *Connection {
	return &Connection{
		ln:     nil,
		conn:   nil,
		reader: nil,
		writer: nil,
	}
}

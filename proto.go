package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type Proto struct {
	bs   int
	conn *Connection
}

const (
	OpFile byte = 1
	OpDone byte = 2
)

var (
	ErrUnknownOpcode = errors.New("proto: unknown opcode")
)

func (proto *Proto) ReadFile(path string, nl func(name string, size int64), pl func(total int64)) (bool, error) {
	t, err := proto.conn.reader.ReadByte()
	if err != nil {
		return false, errors.New("unable to read type")
	}
	switch t {
	case OpFile:
		line, _, err := proto.conn.reader.ReadLine()
		if err != nil {
			return false, errors.New("unable to read name")
		}
		var size int64
		err = binary.Read(proto.conn.reader, binary.LittleEndian, &size)
		if err != nil {
			return false, errors.New("unable to read size")
		}

		name := filepath.Base(string(line))
		nl(name, size)
		file, err := os.Create(filepath.Join(path, name))
		if err != nil {
			return false, errors.New("unable to create file")
		}

		buffer := make([]byte, proto.bs)
		var total int64 = 0
		for {
			if total+int64(len(buffer)) > size {
				buffer = make([]byte, size-total)
			}
			n, err := proto.conn.reader.Read(buffer)
			if err != nil {
				if err == io.EOF {
					break
				}
				return false, errors.New("file read error")
			}
			total += int64(n)
			if _, err = file.Write(buffer[:n]); err != nil {
				return false, errors.New("file write error")
			}
			pl(total)
			if total == size {
				break
			}
		}
		if err = file.Close(); err != nil {
			return false, errors.New("file close error")
		}
		return true, nil
	case OpDone:
		return false, nil
	default:
		return false, ErrUnknownOpcode
	}
}

func (proto *Proto) SendFile(name string, ready func(size int64), l func(total int64)) error {
	base := filepath.Base(name)
	stat, err := os.Stat(name)
	if err != nil {
		return errors.New("unable to get file info")
	}
	size := stat.Size()
	err = proto.conn.writer.WriteByte(OpFile)
	if err != nil {
		return errors.New("event type sending failed")
	}
	ready(size)
	_, err = proto.conn.writer.WriteString(base + "\n")
	if err != nil {
		return errors.New("file name sending failed")
	}

	buf := new(bytes.Buffer)
	if err = binary.Write(buf, binary.LittleEndian, size); err != nil {
		return errors.New("file size preparing failed")
	}
	if _, err = buf.WriteTo(proto.conn.writer); err != nil {
		return errors.New("file size sending failed")
	}

	if err = proto.conn.writer.Flush(); err != nil {
		return errors.New("header flushing failed")
	}

	file, err := os.Open(name)
	if err != nil {
		return errors.New("unable to open file")
	}
	buffer := make([]byte, proto.bs)
	var total int64 = 0
	for {
		n, err := file.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			return errors.New("local file read error")
		}
		total += int64(n)
		if _, err = proto.conn.writer.Write(buffer[:n]); err != nil {
			return errors.New("file write to socket error")
		}
		if err = proto.conn.writer.Flush(); err != nil {
			return errors.New("file data flushing error")
		}
		l(total)
	}
	if err = proto.conn.writer.Flush(); err != nil {
		return errors.New("data flushing error")
	}
	if err = file.Close(); err != nil {
		return errors.New("file closing error")
	}
	return nil
}

func (proto *Proto) SendDone() error {
	if err := proto.conn.writer.WriteByte(OpDone); err != nil {
		return err
	}
	if err := proto.conn.writer.Flush(); err != nil {
		return err
	}
	return nil
}

func NewProto(conn *Connection) *Proto {
	return &Proto{
		bs:   10240,
		conn: conn,
	}
}

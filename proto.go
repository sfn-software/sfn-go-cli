package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Proto struct {
	bs   int
	conn *Connection
}

const (
	OpFile         byte = 1
	OpDone         byte = 2
	OpMD5WithFile  byte = 3
	OpFileWithMD5  byte = 4
	OpFileWithPath byte = 5

	ProtocolPathSeparator = "/"
)

var (
	ErrUnknownOpcode = errors.New("unknown opcode")
	ErrInvalidMD5Sum = errors.New("md5 is invalid")
)

// mkdirp creates a directory, also creating missing parent folders if needed, like `mkdir -p` does.
func mkdirp(baseDir string, relPath string) (absDir string, err error) {
	if len(relPath) == 0 {
		absDir = baseDir
	} else if strings.HasPrefix(relPath, ProtocolPathSeparator) {
		err = errors.New("path can't start with separator")
	} else if strings.HasSuffix(relPath, ProtocolPathSeparator) {
		err = errors.New("path can't end with separator")
	} else {
		absDir = filepath.Join(baseDir, relPath)
		err = os.MkdirAll(absDir, os.ModePerm)
	}
	return
}

func (proto *Proto) ReadFile(baseDir string, nl func(relDir string, name string, size int64), pl func(total int64)) (bool, error) {
	opcode, err := readOpcode(proto.conn.reader)
	if err != nil {
		return false, err
	}
	switch opcode {
	case OpFile:
		name, size, err := readFileNameAndSize(proto.conn.reader)
		if err != nil {
			return false, err
		}
		nl("", name, size)
		if _, err = readFileContents(proto.conn.reader, proto.bs, baseDir, name, size, false, pl); err != nil {
			return false, err
		}
		return true, nil
	case OpMD5WithFile:
		name, size, err := readFileNameAndSize(proto.conn.reader)
		if err != nil {
			return false, err
		}
		nl("", name, size)
		origSum, err := readFileMD5(proto.conn.reader)
		if err != nil {
			return false, err
		}
		sum, err := readFileContents(proto.conn.reader, proto.bs, baseDir, name, size, false, pl)
		if err != nil {
			return false, err
		}
		if sum != origSum {
			return true, ErrInvalidMD5Sum
		}
		return true, nil
	case OpFileWithMD5:
		name, size, err := readFileNameAndSize(proto.conn.reader)
		if err != nil {
			return false, err
		}
		nl("", name, size)
		sum, err := readFileContents(proto.conn.reader, proto.bs, baseDir, name, size, false, pl)
		if err != nil {
			return false, err
		}
		origSum, err := readFileMD5(proto.conn.reader)
		if err != nil {
			return false, err
		}
		if sum != origSum {
			return true, ErrInvalidMD5Sum
		}
		return true, nil
	case OpFileWithPath:
		name, size, err := readFileNameAndSize(proto.conn.reader)
		if err != nil {
			return false, err
		}
		relPath, err := readFilePath(proto.conn.reader)
		if err != nil {
			return false, err
		}
		relPath = filepath.FromSlash(relPath)
		nl(relPath, name, size)
		absDir, err := mkdirp(baseDir, relPath)
		if err != nil {
			return false, err
		}
		executable, err := readExecutableBit(proto.conn.reader)
		if err != nil {
			return false, err
		}
		sum, err := readFileContents(proto.conn.reader, proto.bs, absDir, name, size, executable, pl)
		if err != nil {
			return false, err
		}
		origSum, err := readFileMD5(proto.conn.reader)
		if err != nil {
			return false, err
		}
		if sum != origSum {
			return true, ErrInvalidMD5Sum
		}
		return true, nil
	case OpDone:
		return false, nil
	default:
		return false, ErrUnknownOpcode
	}
}

func readOpcode(reader *bufio.Reader) (byte, error) {
	opcode, err := reader.ReadByte()
	if err != nil {
		return 0, errors.New("unable to read type")
	}
	return opcode, nil
}

func readFileNameAndSize(reader *bufio.Reader) (name string, size int64, err error) {
	name, err = readFileName(reader)
	if err != nil {
		return
	}
	size, err = readFileSize(reader)
	if err != nil {
		return
	}
	return
}

func readFileName(reader *bufio.Reader) (name string, err error) {
	line, _, err := reader.ReadLine()
	if err != nil {
		err = errors.New("unable to read name")
		return
	}
	name = filepath.Base(string(line))
	return
}

func readFilePath(reader *bufio.Reader) (ret string, err error) {
	line, _, err := reader.ReadLine()
	if err != nil {
		err = errors.New("unable to read path")
		return
	}
	ret = string(line)
	return
}

func readFileSize(reader *bufio.Reader) (size int64, err error) {
	err = binary.Read(reader, binary.LittleEndian, &size)
	if err != nil {
		err = errors.New("unable to read size")
		return
	}
	return
}

func readFileMD5(reader *bufio.Reader) (name string, err error) {
	line, _, err := reader.ReadLine()
	if err != nil {
		err = errors.New("unable to read MD5")
		return
	}
	name = string(line)
	return
}

func readExecutableBit(reader *bufio.Reader) (executable bool, err error) {
	b, err := reader.ReadByte()
	if err != nil {
		err = errors.New("unable to read executable bit")
		return
	}
	executable = b == 1
	return
}

func readFileContents(reader *bufio.Reader, bufferSize int, path string, name string, size int64, executable bool, pl func(total int64)) (string, error) {
	file, err := os.Create(filepath.Join(path, name))
	if err != nil {
		return "", errors.New("unable to create file")
	}
	if executable {
		stat, err := file.Stat()
		if err != nil {
			return "", errors.New("unable to get file info")
		}
		mode := SetExecAny(stat.Mode())
		if err = file.Chmod(mode); err != nil {
			return "", errors.New("unable to set file mode")
		}
	}
	//goland:noinspection GoUnhandledErrorResult
	defer file.Close()

	h := md5.New()
	input := io.TeeReader(reader, h)
	buffer := make([]byte, bufferSize)
	var total int64 = 0
	for {
		if total+int64(len(buffer)) > size {
			buffer = make([]byte, size-total)
		}
		n, err := input.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", errors.New("file read error")
		}
		total += int64(n)
		if _, err = file.Write(buffer[:n]); err != nil {
			return "", errors.New("file write error")
		}
		pl(total)
		if total == size {
			break
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (proto *Proto) SendFile(baseDir string, filePath string, ready func(relDir string, size int64), l func(total int64)) error {
	fileName := filepath.Base(filePath)
	relDir, err := filepath.Rel(baseDir, filepath.Dir(filePath))
	if err != nil {
		return err
	} else if relDir == "." {
		relDir = ""
	}
	stat, err := os.Stat(filePath)
	if err != nil {
		return errors.New("unable to get file info")
	}
	size := stat.Size()
	var executable byte
	if IsExecAny(stat.Mode()) {
		executable = 1
	}
	err = proto.conn.writer.WriteByte(OpFileWithPath)
	if err != nil {
		return errors.New("event type sending failed")
	}
	ready(relDir, size)
	_, err = proto.conn.writer.WriteString(fileName + "\n")
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

	_, err = proto.conn.writer.WriteString(relDir + "\n")
	if err != nil {
		return errors.New("file path sending failed")
	}

	err = proto.conn.writer.WriteByte(executable)
	if err != nil {
		return errors.New("file mode sending failed")
	}

	if err = proto.conn.writer.Flush(); err != nil {
		return errors.New("header flushing failed")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return errors.New("unable to open file")
	}
	//goland:noinspection GoUnhandledErrorResult
	defer file.Close()

	h := md5.New()
	input := io.TeeReader(file, h)
	buffer := make([]byte, proto.bs)
	var total int64 = 0
	for {
		n, err := input.Read(buffer)
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

	sum := fmt.Sprintf("%x", h.Sum(nil))
	_, err = proto.conn.writer.WriteString(sum + "\n")
	if err != nil {
		return errors.New("md5 sending failed")
	}

	if err = proto.conn.writer.Flush(); err != nil {
		return errors.New("data flushing error")
	}
	return nil
}

func SetExecAny(mode os.FileMode) os.FileMode {
	return mode | 0111
}

func IsExecAny(mode os.FileMode) bool {
	return mode&0111 != 0
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

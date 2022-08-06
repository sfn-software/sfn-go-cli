package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const VersionMajor = 1
const VersionMinor = 0

type fileEntity struct {
	filePath string
	baseDir  string
}

func main() {
	hostPtr := flag.String("connect", "", "Host address")
	listenPtr := flag.Bool("listen", false, "Listen for connection")
	portPtr := flag.Int("port", 3214, "Connection port")
	helpPtr := flag.Bool("help", false, "Show help")
	versionPtr := flag.Bool("version", false, "Show version")
	dirPtr := flag.String("dir", "", "Directory for receiving files")

	flag.Parse()

	dir := filepath.Clean(*dirPtr)

	if helpPtr != nil && *helpPtr == true {
		flag.Usage()
		return
	}

	if versionPtr != nil && *versionPtr == true {
		fmt.Printf("Version: %d.%d\n", VersionMajor, VersionMinor)
		return
	}

	var files []fileEntity
	args := flag.Args()
	if len(args) > 0 {
		for _, arg := range args {
			stat, err := os.Stat(arg)
			if err != nil {
				fmt.Println(Colored("✘ Unable to open %s", ColorRed, arg))
				continue
			}
			if stat.IsDir() {
				dirFiles, err := scanDir(filepath.Clean(arg))
				if err != nil {
					fmt.Println(Colored("✘ Unable to scan dir %s", ColorRed, arg))
					continue
				}
				files = append(files, dirFiles...)
			} else {
				e := fileEntity{
					filePath: filepath.Clean(arg),
					baseDir:  filepath.Dir(arg),
				}
				files = append(files, e)
			}
		}
	}

	conn := NewConnection()

	address := fmt.Sprintf("%s:%d", *hostPtr, *portPtr)
	if hostPtr != nil && *hostPtr != "" {
		fmt.Println(Colored("☛ Connecting to %s", ColorCyan, address))
		if _, err := conn.Connect(address); err == nil {
			defer safeDisconnect(conn)
			fmt.Println(Colored("⇄ Connected", ColorCyan))
			processFiles(conn, files, dir)
			fmt.Println(Colored("⇵ Transfer done", ColorCyan))
		} else {
			fmt.Println(Colored("✘ Unable to connect to %s", ColorRed, address))
		}
	} else if listenPtr != nil && *listenPtr {
		fmt.Println(Colored("☛ Listening...", ColorCyan))
		if _, err := conn.Listen(address); err == nil {
			defer safeDisconnect(conn)
			fmt.Println(Colored("⇄ Connected", ColorCyan))
			processFiles(conn, files, dir)
			fmt.Println(Colored("⇵ Transfer done", ColorCyan))
		} else {
			fmt.Println(Colored("✘ Unable to listen on %s", ColorRed, address))
		}
	} else {
		flag.Usage()
	}
}

func processFiles(conn *Connection, files []fileEntity, dir string) {
	proto := NewProto(conn)
	for _, file := range files {
		var progress *TtyProgress
		err := proto.SendFile(file.baseDir, file.filePath, func(relDir string, size int64) {
			progress = NewProgress(relDir, filepath.Base(file.filePath), size, Receiving)
		}, func(total int64) {
			progress.Draw(total)
		})
		if err != nil {
			progress.Failed(err)
			fmt.Println()
			return
		} else {
			progress.Done()
			fmt.Println()
		}
	}
	_ = proto.SendDone()
	for {
		var progress *TtyProgress
		hasMore, err := proto.ReadFile(dir, func(relDir string, name string, size int64) {
			progress = NewProgress(relDir, name, size, Sending)
		}, func(total int64) {
			progress.Draw(total)
		})
		if !hasMore {
			break
		}
		if err == ErrInvalidMD5Sum {
			progress.Warning(err)
			fmt.Println()
		} else if err != nil {
			if progress != nil {
				progress.Failed(err)
			} else {
				fmt.Print(Colored("✘ Receive file error: %s", ColorRed, err.Error()))
			}
			fmt.Println()
			return
		} else {
			progress.Done()
			fmt.Println()
		}
	}
}

func safeDisconnect(conn *Connection) {
	err := conn.Disconnect()
	if err != nil {
		fmt.Println(Colored("✘ Disconnection failure", ColorRed))
		return
	}
	fmt.Println(Colored("↮ Disconnected", ColorCyan))
}

func scanDir(base string) ([]fileEntity, error) {
	var files []fileEntity
	err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			e := fileEntity{
				filePath: path,
				baseDir:  base,
			}
			files = append(files, e)
		}
		return nil
	})
	return files, err
}

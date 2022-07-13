package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const VersionMajor = 1
const VersionMinor = 0

func main() {
	hostPtr := flag.String("connect", "", "Host address")
	listenPtr := flag.Bool("listen", false, "Listen for connection")
	portPtr := flag.Int("port", 3214, "Connection port")
	helpPtr := flag.Bool("help", false, "Show help")
	versionPtr := flag.Bool("version", false, "Show version")
	dirPtr := flag.String("dir", "", "Directory for receiving files")

	flag.Parse()

	if helpPtr != nil && *helpPtr == true {
		flag.Usage()
	}

	if versionPtr != nil && *versionPtr == true {
		fmt.Printf("Version: %d.%d\n", VersionMajor, VersionMinor)
	}

	var files []string
	args := flag.Args()
	if len(args) > 0 {
		for _, arg := range args {
			stat, err := os.Stat(arg)
			if err != nil {
				fmt.Println(Colored("✘ Unable to open %s", ColorRed, arg))
				continue
			}
			if stat.IsDir() {
				dirFiles, err := scanDir(arg)
				if err != nil {
					fmt.Println(Colored("✘ Unable to scan dir %s", ColorRed, arg))
					continue
				}
				files = append(files, dirFiles...)
			} else {
				files = append(files, arg)
			}
		}
	}

	conn := NewConnection()

	address := fmt.Sprintf("%s:%d", *hostPtr, *portPtr)
	if hostPtr != nil && *hostPtr != "" {
		fmt.Println(Colored("☛ Connecting to %s", ColorGreen, address))
		if _, err := conn.Connect(address); err == nil {
			defer safeDisconnect(conn)
			fmt.Println(Colored("✔ Connected", ColorGreen))
			processFiles(conn, files, *dirPtr)
		} else {
			fmt.Println(Colored("✘ Unable to connect to %s", ColorRed, address))
		}
	} else if listenPtr != nil {
		fmt.Println(Colored("☛ Listening...", ColorGreen))
		if _, err := conn.Listen(address); err == nil {
			defer safeDisconnect(conn)
			fmt.Println(Colored("✔ Connected", ColorGreen))
			processFiles(conn, files, *dirPtr)
		} else {
			fmt.Println(Colored("✘ Unable to listen on %s", ColorRed, address))
		}
	}
}

func processFiles(conn *Connection, files []string, dir string) {
	proto := NewProto(conn)
	for _, file := range files {
		progress := NewProgress(file, 0, Receiving)
		err := proto.SendFile(file, func(size int64) {
			progress.fileSize = size
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
		hasMore, err := proto.ReadFile(dir, func(name string, size int64) {
			progress = NewProgress(name, size, Sending)
		}, func(total int64) {
			progress.Draw(total)
		})
		if !hasMore {
			break
		}
		if err != nil {
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
	}
}

func scanDir(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

/*
fs handle container fs operation
*/
package cops

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/nitrous-io/dedicated/pkg/airbrake"
)

const (
	textContent = "text"
	pngContent  = "image/png"
	jpgContent  = "image/jpg"
	gifContent  = "image/gif"
	iconContent = "image/x-icon"

	softSizeLimit = 2 * 1024 * 1024  // 2 megabytes
	hardSizeLimit = 20 * 1024 * 1024 // 20 megabytes
)

var (
	ErrFileExist      = errors.New("aio:file_exists")
	ErrFileTooLarge   = errors.New("aio:file_too_large")
	ErrNotTextFile    = errors.New("aio:not_text_file")
	ErrOversizedImage = errors.New("aio:file_is_oversized_image")
)

type FS struct {
}

func (c *FS) HandleAsync(msg *Message) chan *Message {
	resp := make(chan *Message, chanSize)

	switch msg.Event {
	case "list":
		dir := msg.Args[0].(string)
		entries, err := readDir(dir)
		if err != nil {
			return sendErr(resp, msg, err)
		}
		files := make([]interface{}, len(entries))
		for i, f := range entries {
			files[i] = fileInfoArgs(f)
		}
		return sendFinalReply(resp, msg, []interface{}{dir, files})
	case "read":
		fileName := msg.Args[0].(string)
		info, err := c.read(fileName)
		if err != nil {
			return sendErr(resp, msg, err)
		}
		return sendFinalReply(resp, msg, info)
	case "download":
		// FIXME
	case "write":
		fileName := msg.Args[0].(string)
		content := msg.Args[1].(string)
		if err := c.write(fileName, content); err != nil {
			return sendErr(resp, msg, err)
		}
		return sendFinalReply(resp, msg, []interface{}{fileName})
	case "writeNew":
		fileName := msg.Args[0].(string)
		content := msg.Args[1].(string)
		if err := c.writeNew(fileName, content); err != nil {
			return sendErr(resp, msg, err)
		}
		return sendFinalReply(resp, msg, []interface{}{fileName})
	case "makeDir":
		dirName := msg.Args[0].(string)
		if err := c.mkdir(dirName); err != nil {
			return sendErr(resp, msg, err)
		}
		return sendFinalReply(resp, msg, []interface{}{dirName})
	case "deleteDir":
		dirName := msg.Args[0].(string)
		if err := c.rmdir(dirName); err != nil {
			return sendErr(resp, msg, err)
		}
		return sendFinalReply(resp, msg, []interface{}{dirName})
	case "deleteFile":
		filename := msg.Args[0].(string)
		if err := c.deleteFile(filename); err != nil {
			return sendErr(resp, msg, err)
		}
		return sendFinalReply(resp, msg, []interface{}{filename})
	case "rename":
		oldPath := msg.Args[0].(string)
		newPath := msg.Args[1].(string)
		if err := c.rename(oldPath, newPath); err != nil {
			return sendErr(resp, msg, err)
		}
		return sendFinalReply(resp, msg, []interface{}{oldPath})
	case "upload":
		// FIXME
	case "unzip":
		// FIXME
	}
	close(resp)
	return resp
}

func (c *FS) rmdir(dirName string) error {
	return os.RemoveAll(dirName)
}

func (c *FS) rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (c *FS) deleteFile(fileName string) error {
	return os.Remove(fileName)
}

func (c *FS) mkdir(dir string) error {
	stat, err := os.Stat(filepath.Dir(dir))
	if err != nil {
		return err
	}
	uid, gid, err := owner(stat)
	if err != nil {
		return err
	}
	if err := os.Mkdir(dir, os.ModeDir|0755); err != nil {
		return err
	}
	return os.Chown(dir, uid, gid)
}

func (c *FS) write(path, content string) error {
	f, err := openFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, bufio.NewReader(strings.NewReader(content)))
	return err
}

func (c *FS) writeNew(path, content string) error {
	f, err := openFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, bufio.NewReader(strings.NewReader(content)))
	return err
}

func (c *FS) read(path string) ([]interface{}, error) {
	// open the file
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// get stat of the file
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// check if the size is too large
	if stat.Size() > hardSizeLimit {
		return nil, ErrFileTooLarge
	}
	// get content of the file
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	// trying to guess content type
	var contentType string
	if stat.Size() == 0 {
		contentType = textContent
	} else {
		contentType, err = detectContentType(data)
		if err != nil {
			return nil, err
		}
	}
	// from the content type, encode data properly
	var content string
	switch contentType {
	case textContent:
		content = string(data)
	case pngContent, gifContent, jpgContent, iconContent:
		if stat.Size() > softSizeLimit {
			return nil, ErrOversizedImage
		}
		content = base64.StdEncoding.EncodeToString(data)
	}
	return []interface{}{path, contentType, content}, nil
}

func NewFsHandler() AsyncHandler {
	return &FS{}
}

func fileInfoArgs(info os.FileInfo) []interface{} {
	return []interface{}{
		info.Name(),
		info.IsDir(),
		info.Size(),
	}
}

func detectContentType(data []byte) (string, error) {
	var contentType string
	detectedContent := http.DetectContentType(data)
	// if the file is a text file or empty
	if strings.Contains(detectedContent, "text") {
		contentType = textContent
	} else if detectedContent == "image/png" {
		contentType = pngContent
	} else if detectedContent == "image/gif" {
		contentType = gifContent
	} else if detectedContent == "image/jpeg" {
		contentType = jpgContent
	} else if strings.Contains(detectedContent, "icon") {
		contentType = iconContent
	}
	if len(contentType) == 0 {
		return "", ErrNotTextFile
	}
	return contentType, nil
}

func openFile(p string, flag int) (*os.File, error) {
	stat, err := os.Stat(filepath.Dir(p))
	if err != nil {
		return nil, err
	}
	uid, gid, err := owner(stat)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, flag, 0755)
	if err != nil {
		return nil, err
	}
	if err := f.Chown(uid, gid); err != nil {
		return nil, err
	}
	return f, nil
}

func owner(stat os.FileInfo) (int, int, error) {
	s, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("failed to get system file stat")
	}
	return int(s.Uid), int(s.Gid), nil
}

func readDir(name string) (fi []os.FileInfo, err error) {
	f, err := os.Open(name)
	if err != nil {
		airbrake.Printf("os.Open('%s') failed with '%s'\n", name, err)
		return nil, err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	fi = make([]os.FileInfo, 0, len(names))
	for _, filename := range names {
		// use stat to read symlink as well
		fp := filepath.Clean(filepath.Join(name, filename))
		fip, lerr := os.Stat(fp)
		if os.IsNotExist(lerr) {
			// File disappeared between readdir + stat.
			// Just treat it as if it didn't exist.
			continue
		}
		if lerr != nil {
			return fi, lerr
		}
		fi = append(fi, fip)
	}
	sort.Sort(byName(fi))
	return fi, err
}

// byName implements sort.Interface.
type byName []os.FileInfo

func (f byName) Len() int           { return len(f) }
func (f byName) Less(i, j int) bool { return f[i].Name() < f[j].Name() }
func (f byName) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

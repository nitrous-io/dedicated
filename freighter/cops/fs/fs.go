/*
fs handle container fs operation
*/
package cfs

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/nitrous-io/dedicated/freighter/cops"
	"github.com/nitrous-io/dedicated/pkg/rootfs"
)

const (
	textContent = "text"
	pngContent  = "image/png"
	jpgContent  = "image/jpg"
	gifContent  = "image/gif"
	iconContent = "image/x-icon"
	sizeLimit   = 2 * 1024 * 1024 // 2 megabytes
)

var (
	ErrFileExist    = errors.New("aio:file_exists")
	ErrFileTooLarge = errors.New("aio:file_too_large")
	ErrNotTextFile  = errors.New("aio:not_text_file")

	RootPrefix = "/var/lib/docker/aufs/mnt"
)

type FS struct {
	id string
	fs *rootfs.Rootfs
}

func (c *FS) Handle(msg *cops.Message) *cops.Message {
	switch msg.Event {
	case "list":
		dir := msg.Args[0].(string)
		entries, err := c.list(dir)
		if err != nil {
			return makeErr(msg, err)
		}
		files := make([]interface{}, len(entries))
		for i, f := range entries {
			files[i] = fileInfoArgs(f)
		}
		return makeReply(msg, files)
	case "read":
		fileName := msg.Args[0].(string)
		info, err := c.read(fileName)
		if err != nil {
			return makeErr(msg, err)
		}
		return makeReply(msg, info)
	case "download":
		// FIXME
	case "write":
		fileName := msg.Args[0].(string)
		content := msg.Args[1].(string)
		if err := c.write(fileName, content); err != nil {
			return makeErr(msg, err)
		}
		return makeReply(msg, []interface{}{fileName})
	case "writeNew":
		fileName := msg.Args[0].(string)
		content := msg.Args[1].(string)
		if err := c.writeNew(fileName, content); err != nil {
			return makeErr(msg, err)
		}
		return makeReply(msg, []interface{}{fileName})
	case "makeDir":
		dirName := msg.Args[0].(string)
		if err := c.mkdir(dirName); err != nil {
			return makeErr(msg, err)
		}
		return makeReply(msg, []interface{}{dirName})
	case "deleteDir":
		dirName := msg.Args[0].(string)
		if err := c.rmdir(dirName); err != nil {
			return makeErr(msg, err)
		}
		return makeReply(msg, []interface{}{dirName})
	case "deleteFile":
		filename := msg.Args[0].(string)
		if err := c.deleteFile(filename); err != nil {
			return makeErr(msg, err)
		}
		return makeReply(msg, []interface{}{filename})
	case "rename":
		oldPath := msg.Args[0].(string)
		newPath := msg.Args[1].(string)
		if err := c.rename(oldPath, newPath); err != nil {
			return makeErr(msg, err)
		}
		return makeReply(msg, []interface{}{oldPath})
	case "upload":
		// FIXME
	case "unzip":
		// FIXME
	}
	return nil
}

func (c *FS) rmdir(dirName string) error {
	return c.fs.Remove(cops.GetPath(dirName))
}

func (c *FS) rename(oldPath, newPath string) error {
	return c.fs.Rename(cops.GetPath(oldPath), cops.GetPath(newPath))
}

func (c *FS) deleteFile(fileName string) error {
	return c.fs.Remove(cops.GetPath(fileName))
}

func (c *FS) mkdir(dir string) error {
	return c.fs.Mkdir(cops.GetPath(dir))
}

func (c *FS) write(path, content string) error {
	f, err := c.fs.Create(cops.GetPath(path))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, bufio.NewReader(strings.NewReader(content)))
	return err
}

func (c *FS) writeNew(path, content string) error {
	f, err := c.fs.CreateNew(cops.GetPath(path))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, bufio.NewReader(strings.NewReader(content)))
	return err
}

func (c *FS) list(dir string) ([]os.FileInfo, error) {
	return c.fs.ReadDir(dir)
}

func (c *FS) read(path string) ([]interface{}, error) {
	// open the file
	f, err := c.fs.Open(cops.GetPath(path))
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
	if stat.Size() > sizeLimit {
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
		contentType, err = getContent(data)
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
		content = base64.StdEncoding.EncodeToString(data)
	}
	return []interface{}{path, contentType, content}, nil
}

func NewFS(id string) (cops.Handler, error) {
	root := path.Join(RootPrefix, id)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to find rootfs for %s, %s", id, err)
	}
	return &FS{
		id: id,
		fs: rootfs.NewRootfs(root),
	}, nil
}

func init() {
	cops.RegisterHandler("fs", NewFS)
}

func makeErr(orig *cops.Message, err error) *cops.Message {
	return &cops.Message{
		Namespace: orig.Namespace,
		Event:     orig.Event,
		CID:       orig.CID,
		Error:     err,
	}
}

func makeReply(orig *cops.Message, args []interface{}) *cops.Message {
	return &cops.Message{
		Namespace: orig.Namespace,
		Event:     orig.Event,
		CID:       orig.CID,
		Args:      args,
	}
}

func fileInfoArgs(info os.FileInfo) []interface{} {
	return []interface{}{
		info.Name(),
		info.IsDir(),
		info.Size(),
	}
}

func getContent(data []byte) (string, error) {
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

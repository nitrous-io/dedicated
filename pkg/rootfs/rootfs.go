package rootfs

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

var (
	ErrInvalidPath = errors.New("invalid path")
)

type Rootfs struct {
	root string
}

func (r *Rootfs) Create(p string) (*os.File, error) {
	return r.openFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC)
}

func (r *Rootfs) CreateNew(p string) (*os.File, error) {
	return r.openFile(p, os.O_RDWR|os.O_CREATE|os.O_EXCL)
}

func (r *Rootfs) Open(p string) (*os.File, error) {
	p, err := r.path(p)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (r *Rootfs) Mkdir(p string) error {
	p, err := r.path(p)
	if err != nil {
		return err
	}
	stat, err := os.Stat(filepath.Dir(p))
	if err != nil {
		return err
	}
	uid, gid, err := owner(stat)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p, 0755); err != nil {
		return err
	}
	return os.Chown(p, uid, gid)
}

func (r *Rootfs) Rename(oldName, newName string) (err error) {
	oldName, err = r.path(oldName)
	if err != nil {
		return err
	}
	newName, err = r.path(newName)
	if err != nil {
		return err
	}
	return os.Rename(oldName, newName)
}

func (r *Rootfs) Remove(name string) error {
	p, err := r.path(name)
	if err != nil {
		return err
	}
	return os.RemoveAll(p)
}

func (r *Rootfs) ReadDir(name string) ([]os.FileInfo, error) {
	p, err := r.path(name)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadDir(p)
}

func (r *Rootfs) Stat(name string) (os.FileInfo, error) {
	p, err := r.path(name)
	if err != nil {
		return nil, err
	}
	return os.Stat(p)
}

func (r *Rootfs) openFile(p string, flag int) (*os.File, error) {
	p, err := r.path(p)
	if err != nil {
		return nil, err
	}
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

func (r *Rootfs) path(p string) (string, error) {
	cp := filepath.Clean(path.Join(r.root, p))
	if strings.HasPrefix(cp, filepath.Clean(r.root+"/")) {
		return cp, nil
	}
	return "", ErrInvalidPath
}

func NewRootfs(root string) *Rootfs {
	return &Rootfs{root: root}
}

func owner(stat os.FileInfo) (int, int, error) {
	s, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("failed to get system file stat")
	}
	return int(s.Uid), int(s.Gid), nil
}

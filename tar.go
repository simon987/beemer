package main

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Tar struct {
	FileCount int
	Name      string
	writer    *tar.Writer
}

func NewTar(path string) (*Tar, error) {

	fp, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		return nil, err
	}

	writer := tar.NewWriter(fp)

	return &Tar{
		0,
		path,
		writer,
	}, nil
}

func getTarPath(tempDir string) string {
	return filepath.Join(tempDir, time.Now().Format("2006-01-02 15.04.05.000.tar"))
}

func (t *Tar) AddFile(path string) error {

	// Header
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(fi, "")
	if err != nil {
		return err
	}
	err = t.writer.WriteHeader(header)
	if err != nil {
		return err
	}

	// Content
	in, err := os.Open(path)
	if err != nil {
		return err
	}

	_, err = io.Copy(t.writer, in)
	if err != nil {
		return err
	}

	t.FileCount += 1

	return nil
}

func (t *Tar) Close() {
	_ = t.writer.Close()
}

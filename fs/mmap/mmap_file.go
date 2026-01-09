package mmap

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/edsrzf/mmap-go"
	"go.uber.org/zap"
)

type MMappedFile struct {
	f io.Closer
	m mmap.MMap
}

func (m *MMappedFile) Close() error {
	err := m.m.Unmap()
	if err != nil {
		err = fmt.Errorf("mmappedFile: unmapping: %w", err)
	}

	if fErr := m.f.Close(); fErr != nil {
		return errors.Join(fmt.Errorf("close mmappedFile.f: %w", fErr), err)
	}

	return err
}

func GetMMappedFile(filename string, filesize int, logger *zap.Logger) ([]byte, io.Closer, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o666)
	if err != nil {
		absPath, pathErr := filepath.Abs(filename)
		if pathErr != nil {
			absPath = filename
		}
		logger.Error("mmappedFile: open", zap.String("path", absPath), zap.Error(err))
		return nil, nil, fmt.Errorf("mmappedFile: open: %w", err)
	}

	err = file.Truncate(int64(filesize))
	if err != nil {
		file.Close()
		logger.Error("mmappedFile: truncate", zap.String("filename", filename), zap.Error(err))
		return nil, nil, fmt.Errorf("mmappedFile: truncate: %w", err)
	}

	fileAsBytes, err := mmap.Map(file, mmap.RDWR, 0)
	if err != nil {
		file.Close()
		logger.Error("mmappedFile: mmap", zap.String("filename", filename),
			zap.Int("Attempted size", filesize), zap.Error(err))
		return nil, nil, fmt.Errorf("mmappedFile: mmap: %w", err)
	}

	return fileAsBytes, &MMappedFile{file, fileAsBytes}, nil
}

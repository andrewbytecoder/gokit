package fileutil

import (
	"os"
	"path/filepath"
)

func DirSize(dir string) (int64, error) {
	var size int64
	err := filepath.Walk(dir, func(filePath string, info os.FileInfo, err error) error {
		// 有任意一个错误出现，则返回
		if err != nil {
			return err
		}

		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

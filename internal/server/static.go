package server

import (
	"io/fs"
	"mime"
	"net/http"
)

// Go's built-in table has no .webmanifest entry and falls back to the host's
// mime.types, which the image may not have.
func init() {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		panic(err)
	}
}

// filesOnly serves files but reports directories as missing, so /static/
// and its subdirectories do not list their contents.
type filesOnly struct{ http.FileSystem }

func (f filesOnly) Open(name string) (http.File, error) {
	file, err := f.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		_ = file.Close()
		if err == nil {
			err = fs.ErrNotExist
		}
		return nil, err
	}
	return file, nil
}

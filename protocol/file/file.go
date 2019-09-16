package file

import (
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"

	"github.com/mjibson/moggio/codec"
	"github.com/mjibson/moggio/protocol"
	"golang.org/x/oauth2"
)

func init() {
	protocol.Register("file", []string{"directory"}, New, reflect.TypeOf(&File{}))
	gob.Register(new(File))
}

func New(params []string, token *oauth2.Token) (protocol.Instance, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("expected one parameter")
	}
	p, err := filepath.Abs(params[0])
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	f.Close()
	return &File{
		Path:  p,
		Songs: make(protocol.SongList),
	}, nil
}

type File struct {
	Path  string
	Songs protocol.SongList
}

func (f *File) Key() string {
	return f.Path
}

func (f *File) Info(id codec.ID) (*codec.SongInfo, error) {
	if _, ok := f.Songs[id]; !ok {
		if _, err := f.List(); err != nil {
			return nil, err
		}
	}
	v := f.Songs[id]
	if v == nil {
		return nil, fmt.Errorf("could not find %v", id)
	}
	return v, nil
}

func (f *File) GetSong(id codec.ID) (codec.Song, error) {
	top, child := id.Pop()
	return codec.ByExtensionID(top, child, fileReader(top))
}

func (f *File) List() (protocol.SongList, error) {
	if len(f.Songs) == 0 {
		return f.Refresh()
	}
	return f.Songs, nil
}

/*
IsCover checks if the filename contained in "path" is a legal name for a cover image.
*/
func IsCover(path string) bool {
	match, _ := regexp.MatchString("(?i)(cover|folder)\\.(jpg|jpeg|png)$", path)
	return match
}

func (f *File) Refresh() (protocol.SongList, error) {
	covers := make(map[string]string)
	songs := make(protocol.SongList)

	// first pass: get covers
	err := filepath.Walk(f.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !IsCover(path) {
			return nil
		}

		/*data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}*/
		// this is actually a workaround - we could theoretically use "file://" URLs, but browsers deny access to local files by default nowadays :(
		//covers[filepath.Dir(path)] = fmt.Sprintf("data:%s;base64,%s", "img/jpeg", base64.StdEncoding.EncodeToString(data))
		covers[filepath.Dir(path)] = fmt.Sprintf("/cover/" + base64.StdEncoding.EncodeToString([]byte(path)))

		return nil
	})

	// second pass: scan music files, add covers and other info
	err = filepath.Walk(f.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		ss, _, err := codec.ByExtension(path, fileReader(path))
		if err != nil || len(ss) == 0 {
			return nil
		}
		for i, s := range ss {
			info, _ := s.Info()
			if info.Title == "" {
				title := filepath.Base(path)
				if len(ss) != 1 {
					title += fmt.Sprintf(":%v", i)
				}
				info.Title = title
			}
			if info.Album == "" {
				info.Album = filepath.Base(filepath.Dir(path))
			}
			cover, exists := covers[filepath.Dir(path)]
			if exists {
				info.ImageURL = cover
			}
			songs[codec.NewID(path, string(i))] = &info
		}
		return nil
	})
	f.Songs = songs
	return songs, err
}

func fileReader(path string) codec.Reader {
	return func() (io.ReadCloser, int64, error) {
		log.Println("open file", path)
		f, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, 0, err
		}
		return f, fi.Size(), nil
	}
}

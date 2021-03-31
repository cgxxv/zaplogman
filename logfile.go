package logman

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type logfile struct {
	filename string //The real log filename for log written.
	logtime  time.Time

	buff  *bufio.Writer
	start sync.Once

	m *Man
}

const _size = 0 //8kb

func (l *logfile) exit() {
	for {
		time.Sleep(1 * time.Second)

		info, err := os.Stat(l.filename)
		if err != nil {
			log.Println(err)
			continue
		}
		if time.Now().Sub(info.ModTime()) < 1*time.Second {
			continue
		}

		l.m.mu.Lock()
		if err := l.close(); err != nil {
			log.Printf("close file [%s] error: %v", l.filename, err)
		}
		delete(l.m.logfiles, l.filename)
		delete(l.m.filemap, l.logtime)
		l.m.mu.Unlock()
		log.Printf("quit: %s", l.filename)
		l.m.wg.Done()

		break
	}
}

func (l *logfile) write(p []byte) (int, error) {
	if l.buff == nil {
		if err := l.openExistingOrNew(); err != nil {
			log.Println(err)
			return 0, err
		}
	}

	n, err := l.buff.Write(p)
	if err != nil {
		log.Println(err)
	}

	return n, err
}

func (l *logfile) openExistingOrNew() error {
	info, err := os.Stat(l.filename)
	if err != nil {
		if os.IsNotExist(err) {
			return l.openNew()
		}
		log.Println(err)
		return err
	}

	f, err := os.OpenFile(l.filename, os.O_APPEND|os.O_WRONLY, info.Mode())
	if err != nil {
		log.Println(err)
		return err
	}

	l.buff = bufio.NewWriterSize(f, _size)

	return nil
}

func (l *logfile) openNew() error {
	dir := filepath.Dir(l.filename)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		log.Println(err)
		return err
	}

	f, err := os.OpenFile(l.filename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
		return err
	}

	l.buff = bufio.NewWriterSize(f, _size)

	return nil
}

func (l *logfile) close() error {
	if l.buff == nil {
		return nil
	}

	err := l.buff.Flush()
	l.buff = nil
	return err
}

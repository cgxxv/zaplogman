package logman

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"
)

const (
	DAILY    = "daily"
	HOURLY   = "hourly"
	MINUTELY = "minutely"
	SECONDLY = "secondly"

	compressSuffix = ".gz"
)

type Man struct {
	Filename   string //The logfile for storing log.
	MaxAge     int    //The maximum days for retain old logfiles.
	MaxBackups int    //The maximun number for ratain raw logfiles.
	Compress   bool   //The switch for compress the old logfiles.
	Timing     string //Such as daily, hourly, minutely, secondly(for testing)
	Level      zapcore.Level

	timeFormat string    //The time format layout.
	logtime    time.Time //Assign just the second, minute, hour, day.
	curtime    time.Time //Assign just the second, minute, hour, day.

	logfiles map[string]*logfile
	filemap  map[time.Time]string

	millCh    chan bool
	startMill sync.Once

	mu sync.Mutex
	wg sync.WaitGroup
}

func (m *Man) waitAll() {
	log.Printf("quit goroutines: %d", runtime.NumGoroutine())
	m.mu.Lock()
	for filename, lf := range m.logfiles {
		lf.close()
		delete(m.logfiles, filename)
		delete(m.filemap, lf.logtime)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Man) dispatch(t time.Time, p []byte) error {
	m.mu.Lock()

	m.setlogtime(t)
	filename := m.filename()

	lf, ok := m.logfiles[filename]
	if !ok {
		lf = m.rotate(filename)
	}

	_, err := lf.write(p)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	return nil
}

func (m *Man) rotate(filename string) *logfile {
	lf := &logfile{
		filename: filename,
		logtime:  m.logtime,
		start:    sync.Once{},
		m:        m,
	}
	lf.start.Do(func() {
		m.wg.Add(1)
		go lf.exit()
	})

	m.logfiles[filename] = lf
	m.curtime = m.logtime

	//compress, remove old logfiles
	m.mill()

	return lf
}

func (m *Man) mill() {
	m.startMill.Do(func() {
		m.millCh = make(chan bool)
		go m.millRun()
	})
	select {
	case m.millCh <- true:
	default:
	}
}

func (m *Man) millRun() {
	for range m.millCh {
		_ = m.millRunOnce()
	}
}

func (m *Man) millRunOnce() error {
	if m.MaxBackups == 0 && m.MaxAge == 0 && !m.Compress {
		return nil
	}

	files, err := m.oldLogFiles()
	if err != nil {
		return err
	}
	// for _, f := range files {
	// 	fmt.Printf("================================%s %s\n", f.timestamp, f.Name())
	// }

	var compress, remove []logInfo

	if m.MaxBackups > 0 && m.MaxBackups < len(files) {
		var preserved int
		var remaining []logInfo
		for _, f := range files {
			// Only count the uncompressed log file or the
			// compressed log file, not both.
			fn := f.Name()
			if strings.HasSuffix(fn, compressSuffix) {
				fn = fn[:len(fn)-len(compressSuffix)]
			}
			preserved++

			if preserved > m.MaxBackups {
				remove = append(remove, f)
			} else {
				remaining = append(remaining, f)
			}
		}
		files = remaining
	}
	// for _, f := range files {
	// 	fmt.Printf("ramaining, excepting(MaxBackups): %s %s\n", f.timestamp, f.Name())
	// }

	if m.MaxAge > 0 {
		diff := time.Duration(int64(24*time.Hour) * int64(m.MaxAge))
		cutoff := time.Now().Add(-1 * diff)

		var remaining []logInfo
		for _, f := range files {
			if f.timestamp.Before(cutoff) {
				remove = append(remove, f)
			} else {
				remaining = append(remaining, f)
			}
		}
		files = remaining
	}
	// for _, f := range files {
	// 	fmt.Printf("ramaining, excepting(MaxAge): %s %s\n", f.timestamp, f.Name())
	// }

	if m.Compress {
		var backups int
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), compressSuffix) {
				backups++
				if backups > m.MaxBackups {
					compress = append(compress, f)
				}
			}
		}
	}

	for _, f := range remove {
		if time.Now().Sub(f.ModTime()) <= 5*time.Minute { //5 minutes is enough?
			continue
		}
		errRemove := os.Remove(filepath.Join(m.dir(), f.Name()))
		if err == nil && errRemove != nil {
			err = errRemove
		}
	}

	for _, f := range compress {
		if time.Now().Sub(f.ModTime()) <= 5*time.Minute { //5 minutes is enough?
			continue
		}
		fn := filepath.Join(m.dir(), f.Name())
		errCompress := compressLogFile(fn, fn+compressSuffix)
		if err == nil && errCompress != nil {
			err = errCompress
		}
	}

	return err
}

func compressLogFile(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		log.Println(err)
		return err
	}
	defer f.Close()

	info, err := os.Stat(src)
	if err != nil {
		log.Println(err)
		return err
	}

	gzf, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
	if err != nil {
		log.Println(err)
		return err
	}
	defer gzf.Close()

	gz := gzip.NewWriter(gzf)
	if _, err := io.Copy(gz, f); err != nil {
		log.Println(err)
		return err
	}
	if err := gz.Close(); err != nil {
		log.Println(err)
		return err
	}
	if err := gzf.Close(); err != nil {
		log.Println(err)
		return err
	}
	if err := os.Remove(src); err != nil {
		log.Println(err)
		return err
	}

	return nil
}

func (m *Man) oldLogFiles() ([]logInfo, error) {
	files, err := ioutil.ReadDir(m.dir())
	if err != nil {
		log.Println(err)
		return nil, err
	}

	var logFiles []logInfo
	prefix, ext := m.prefixAndExt()
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if t, err := m.timeFromName(file.Name(), prefix, ext); err == nil {
			logFiles = append(logFiles, logInfo{t, file})
			continue
		}

		if t, err := m.timeFromName(file.Name(), prefix, ext+compressSuffix); err == nil {
			logFiles = append(logFiles, logInfo{t, file})
			continue
		}
	}

	sort.Sort(byFormatTime(logFiles))

	return logFiles, nil
}

func (m *Man) timeFromName(name, prefix, ext string) (time.Time, error) {
	if !strings.HasPrefix(name, prefix) {
		return time.Time{}, errors.New("mismatched prefix")
	}
	if !strings.HasSuffix(name, ext) {
		return time.Time{}, errors.New("mismatched suffix")
	}
	ts := name[len(prefix) : len(name)-len(ext)]
	return time.Parse(m.getTimeFormat(), ts)
}

func (m *Man) prefixAndExt() (string, string) {
	filename := filepath.Base(m.Filename)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)] + "_"
	return prefix, ext
}

func (m *Man) dir() string {
	return filepath.Dir(m.filename())
}

func (m *Man) filename() string {
	if filename, ok := m.filemap[m.logtime]; ok {
		return filename
	}
	var (
		name string
		dir  string
	)
	if m.Filename != "" {
		name = m.Filename
		dir = filepath.Dir(name)
	} else {
		name = filepath.Base(os.Args[0] + "-log")
		dir, _ = os.Getwd()
		m.Filename = name
	}

	filename := filepath.Base(name)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]
	ts := m.logtime.Format(m.getTimeFormat())
	ret := filepath.Join(dir, fmt.Sprintf("%s_%s%s", prefix, ts, ext))
	m.filemap[m.logtime] = ret
	return ret
}

func (m *Man) setlogtime(t time.Time) {
	s := t.Local().Format(m.getTimeFormat())
	t, _ = time.Parse(m.getTimeFormat(), s)
	m.logtime = t
}

func (m *Man) getTimeFormat() string {
	if m.timeFormat != "" {
		return m.timeFormat
	}

	switch m.Timing {
	case "hourly":
		m.timeFormat = "2006010215"
	case "minutely":
		m.timeFormat = "200601021504"
	case "secondly":
		m.timeFormat = "20060102150405"
	case "daily":
		m.timeFormat = "20060102"
	default:
		m.timeFormat = "20060102"
	}

	return m.timeFormat
}

// logInfo is a convenience struct to return the filename and its embedded
// timestamp.
type logInfo struct {
	timestamp time.Time
	os.FileInfo
}

// byFormatTime sorts by newest time formatted in the name.
type byFormatTime []logInfo

func (b byFormatTime) Less(i, j int) bool {
	return b[i].timestamp.After(b[j].timestamp)
}

func (b byFormatTime) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byFormatTime) Len() int {
	return len(b)
}

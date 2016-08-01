package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	errcode_ok       = 0
	errcode_io_open  = -1
	errcode_io_write = -2
	errcode_io_sync  = -3
	errcode_io_close = -4
	errcode_io_seek  = -4

	errcode_http_invalid_url = 1
	errcode_http_request     = 2
	errcode_http_read        = 3

	errcode_bug = 999
)

type taskInfo struct {
	ID       int   `json:"id"`
	BeginPos int64 `json:"begin"`
	CurPos   int64 `json:"current"`
	EndPos   int64 `json:"end"`

	UseTime   int64 `json:"usetime"`   // 当前为止用了多久时间 单位毫秒
	DealTimes int   `json:"dealtimes"` // 被处理了多少次
	Code      int   `json:"code"`
	Error     error
}

type downloadInfo struct {
	Name     string            `json:"name"`
	Url      string            `json:"url"`
	Header   map[string]string `json:"header"`
	Length   int64             `json:"length"`
	TimeOut  int64             `json:"timeout"`
	TaskList []taskInfo        `json:"tasklist"`
}

func (this *downloadInfo) Parse(data []byte) (err error) {
	err = json.Unmarshal(data, this)
	return
}

func (this *downloadInfo) String() (data []byte, err error) {
	data, err = json.Marshal(this)
	return
}

type ReportInfo struct {
	TotalSize    int64
	CompleteSize int64
	DoneCount    int64
	TaskCount    int64
}

type Downloader struct {
	dbfile       string
	splitLen     int64
	timeout      int64
	threads      int
	isRunning    bool
	mutex        sync.Mutex
	report       ReportInfo
	lastSaveTime time.Time

	data       downloadInfo
	processCh  chan taskInfo // 报告处理进度
	taskqueCh  chan taskInfo // 分发任务
	downloadCh chan taskInfo // 分发任务
}

func NewDownloader(splitLen, timeout int64, threads int) *Downloader {
	return &Downloader{splitLen: splitLen, timeout: timeout, threads: threads}
}

func (this *Downloader) isTaskExist() bool {
	fileinfo, err := os.Stat(this.dbfile)
	if err == nil && !fileinfo.IsDir() {
		return true
	}
	// return err == nil || os.IsExist(err)
	return false
}

func (this *Downloader) Load(dbfile string) error {
	data, err := ioutil.ReadFile(dbfile)
	if err != nil {
		return err
	}

	err = this.data.Parse(data)
	if err != nil {
		return err
	}

	this.dbfile = dbfile

	this.report.TotalSize = this.data.Length

	if this.data.TaskList != nil {
		this.report.TaskCount = int64(len(this.data.TaskList))
		for _, t := range this.data.TaskList {
			this.report.CompleteSize += t.CurPos - t.BeginPos
			if t.CurPos > t.EndPos {
				this.report.DoneCount++
			}
		}
	}

	return nil
}

func (this *Downloader) New(requri, fname, dbfile string, reqheader map[string]string) error {
	var req http.Request
	req.Method = "GET"
	req.Close = true
	downurl, err := url.Parse(requri)
	if err != nil {
		return err
	}
	req.URL = downurl
	header := http.Header{}
	for k, v := range reqheader {
		header.Set(k, v)
	}
	header.Set("Range", fmt.Sprintf("bytes=%d-", 0))
	req.Header = header
	c := http.Client{}
	rsp, err := c.Do(&req)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()

	contentLenStr := rsp.Header.Get("Content-Length")
	fmt.Println(contentLenStr)
	contentLen, err := strconv.ParseInt(contentLenStr, 10, 64)
	if err != nil {
		return err
	}

	if contentLen <= 0 {
		return fmt.Errorf("content len == 0")
	}

	this.dbfile = dbfile
	this.data.Length = contentLen
	this.data.Header = reqheader
	this.data.Name = fname
	this.data.Url = requri
	this.data.TimeOut = this.timeout

	id := 0
	s := int64(0)
	for s < contentLen {
		e := s + this.splitLen
		if e < contentLen {
			task := taskInfo{BeginPos: s, CurPos: s, EndPos: e, ID: id}
			this.data.TaskList = append(this.data.TaskList, task)
			s = e + 1
		} else {
			task := taskInfo{BeginPos: s, CurPos: s, EndPos: contentLen - 1, ID: id}
			this.data.TaskList = append(this.data.TaskList, task)
			break
		}
		id++
	}

	this.report.TotalSize = contentLen
	this.report.CompleteSize = 0

	err = creatFile(this.data.Name, contentLen)
	if err != nil {
		return err
	}
	return this.save(true)
}

func creatFile(filename string, length int64) error {
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, os.FileMode(0644))
	if err != nil {
		return err
	}
	defer f.Close()

	f.Truncate(length)
	err = f.Sync()
	if err != nil {
		return err
	}

	return nil
}

func (this *Downloader) save(force bool) error {
	var err error
	if force || (time.Now().Sub(this.lastSaveTime).Seconds() > 1) {
		data, err := this.data.String()
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(this.dbfile, data, os.FileMode(0644))
		this.lastSaveTime = time.Now()
	}
	return err
}

func (this *Downloader) Start() {
	this.mutex.Lock()
	defer this.mutex.Unlock()

	if !this.isRunning {
		if this.data.TaskList != nil && len(this.data.TaskList) > 0 {
			tasklen := len(this.data.TaskList)
			this.processCh = make(chan taskInfo, tasklen)
			this.taskqueCh = make(chan taskInfo, tasklen)
			this.downloadCh = make(chan taskInfo, tasklen)
			this.report.TaskCount = int64(tasklen)
			go this.hub()
			for i := 0; i < this.threads; i++ {
				go this.handleDownload()
			}
			this.isRunning = true

			for _, t := range this.data.TaskList {
				if t.CurPos <= t.EndPos {
					this.taskqueCh <- t
				}
			}

		}
	}
}

func (this *Downloader) Report() ReportInfo {
	return this.report
}

func (this *Downloader) hub() {
	isProcessChOK := true
	isTaskqueChOK := true
	for isProcessChOK || isTaskqueChOK {
		select {
		case task, ok := <-this.processCh:
			if ok {
				this.handleProcess(task)
			} else {
				isProcessChOK = false
			}
		case task, ok := <-this.taskqueCh:
			if ok {
				this.handleTaskQue(task)
			} else {
				isTaskqueChOK = false
			}
		}
	}

	this.save(true)
}

func (this *Downloader) handleProcess(task taskInfo) {
	old := this.data.TaskList[task.ID]
	this.report.CompleteSize += task.CurPos - old.CurPos
	this.data.TaskList[task.ID] = task
	this.save(false)
}

func (this *Downloader) handleTaskQue(task taskInfo) {
	this.data.TaskList[task.ID] = task
	if task.CurPos <= task.EndPos {
		this.downloadCh <- task
		this.save(false)
	} else {
		// 任务完成
		atomic.AddInt64(&this.report.DoneCount, 1)
		this.save(true)
	}
}

// 启动多个
func (this *Downloader) handleDownload() {
	for task := range this.downloadCh {
		this.downable(task)
	}
}

// 只负责下载
func (this *Downloader) downable(task taskInfo) {
	task.Code = 0
	task.Error = nil
	task.DealTimes++

	usetime := task.UseTime
	starttime := time.Now()
	defer func() {
		task.UseTime = usetime + time.Now().Sub(starttime).Nanoseconds()/1000/1000
		this.taskqueCh <- task
	}()

	f, err := os.OpenFile(this.data.Name, os.O_WRONLY, os.FileMode(0644))
	if err != nil {
		task.Code = errcode_io_open
		task.Error = err
		return
	}

	defer func() {
		f.Close()
	}()

	_, err = f.Seek(task.CurPos, os.SEEK_SET)
	if err != nil {
		task.Code = errcode_io_seek
		task.Error = err
		return
	}

	var req http.Request
	req.Method = "GET"
	req.Close = true
	url, err := url.Parse(this.data.Url)
	if err != nil {
		task.Code = errcode_http_invalid_url
		task.Error = err
		return
	}
	req.URL = url
	header := http.Header{}
	if this.data.Header != nil {
		for k, v := range this.data.Header {
			header.Set(k, v)
		}
	}

	header.Set("Range", fmt.Sprintf("bytes=%d-%d", task.CurPos, task.EndPos))
	req.Header = header

	// TODO 支持cookies

	c := http.Client{}
	c.Timeout = time.Duration(this.data.TimeOut) * time.Second
	rsp, err := c.Do(&req)
	if err != nil {
		task.Code = errcode_http_request
		task.Error = err
		return
	}

	defer rsp.Body.Close()

	buff := make([]byte, 2048)

	for task.CurPos <= task.EndPos {
		n, err := rsp.Body.Read(buff)
		if err == nil || (err == io.EOF && n > 0) {
			if n > 0 {
				wn, err := f.Write(buff[:n])
				if err != nil {
					task.Code = errcode_io_write
					task.Error = err
					break
				} else if wn != n {
					task.Code = errcode_io_write
					task.Error = fmt.Errorf("BUG wn(%d) != n(%d)", wn, n)
					break
				}

				err = f.Sync() // sync失败了,任务建议回退,因为无法确保落地到磁盘。
				if err != nil {
					task.Code = errcode_io_sync
					task.Error = err
					break
				}

				// 报告进度
				task.CurPos += int64(n)
				task.UseTime = usetime + time.Now().Sub(starttime).Nanoseconds()/1000/1000
				this.processCh <- task
			} else {
				task.Code = errcode_bug
				task.Error = fmt.Errorf("BUG: err == nil but read length == 0")
				break
			}
		} else {
			task.Code = errcode_http_read
			task.Error = err
			break
		}
	}
}

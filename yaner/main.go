package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/SteveWarm/spider/downloader"
)

var (
	g_load        = flag.Bool("load", false, "是否加载已经存在的任务")
	g_slice_size  = flag.Int64("S", 1*1024, "分块大小,单位KB")
	g_file_url    = flag.String("url", "", "下载地址")
	g_file_name   = flag.String("name", "", "保存的文件名，不建议带路径")
	g_file_db     = flag.String("db", "", "存储任务进度文件")
	g_timeout     = flag.Int("timeout", 30, "超时时间,单位秒")
	g_threads     = flag.Int("threads", 20, "最大允许多少个线程同时下载")
	g_http_header = flag.String("H", os.Getenv("SPIDER_HEADER"), "请求中加入的header值")
)

func main() {
	loadConfig()
	header := loadHeaderFromFile(*g_http_header)

	var err error

	downloader := downloader.NewDownloader((*g_slice_size)*1024, int64(*g_timeout), *g_threads)
	if *g_load {
		err = downloader.Load(*g_file_db)
	} else {
		err = downloader.New(*g_file_url, *g_file_name, *g_file_db, header)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}

	downloader.Start()

	lastComplete := int64(0)
	for {
		report := downloader.Report()
		if report.TotalSize > 0 {
			speed := float64(report.CompleteSize - lastComplete)
			lastComplete = report.CompleteSize
			fmt.Fprintln(os.Stdout, fmt.Sprintf("[report] %d/%d %d/%d completa: %.2f speed: %f kb/s",
				report.DoneCount,
				report.TaskCount,
				report.CompleteSize,
				report.TotalSize,
				float64(report.CompleteSize)/float64(report.TotalSize),
				speed/1024.0,
			))
			if report.DoneCount >= report.TaskCount {
				break
			} else {
				time.Sleep(time.Second)
			}
		} else {
			fmt.Fprintln(os.Stderr, "BUG: total size < 0")
			break
		}
	}
}

func loadConfig() {
	flag.Parse()
	if *g_load {
		if *g_file_db != "" {
			//ok
		} else {
			flag.Usage()
			os.Exit(1)
		}
	} else {
		if *g_file_url != "" && *g_file_name != "" {
			//ok
		} else {
			flag.Usage()
			os.Exit(1)
		}
	}

	if *g_slice_size <= 0 || *g_timeout <= 0 || *g_threads <= 0 {
		flag.Usage()
		os.Exit(1)
	}

	if *g_file_db == "" {
		*g_file_db = *g_file_name + ".cfg"
	}
}

func loadHeaderFromFile(filename string) map[string]string {
	header := make(map[string]string, 20)
	f, err := os.OpenFile(filename, os.O_RDONLY, os.FileMode(0644))
	if err != nil {
		fmt.Fprintln(os.Stderr, "[loadHeaderFromFile] open file", filename, "err:", err.Error())
		return header
	}
	defer f.Close()

	rownum := 1
	r := bufio.NewReader(f)
	for {
		linebytes, isPrefix, err := r.ReadLine()
		if err == nil && !isPrefix {
			line := strings.TrimSpace(string(linebytes))
			if !strings.HasPrefix(line, "#") {
				hline := strings.SplitN(line, "=", 2)
				if len(hline) == 2 {
					hname := strings.TrimSpace(hline[0])
					hval := strings.TrimSpace(hline[1])
					header[hname] = hval
				} else {
					fmt.Fprintln(os.Stderr, "[loadHeaderFromFile] ignore invalid row. data:", line)
				}
			}
		} else if err != nil {
			if err != io.EOF {
				fmt.Fprintln(os.Stderr, "[loadHeaderFromFile] read err:", err.Error())
			}
			break
		} else {
			fmt.Fprintln(os.Stderr, "[loadHeaderFromFile] ignore too large row. rownum:", rownum)
		}
		rownum++
	}

	return header
}

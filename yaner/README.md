# 说明

使用golang实现的HTTP多线程下载命令行工具。
支持断点续传。
支持存储任务进度，任务终止后可以从上一次成功的地方继续开始下载。

# 安装

```
go get github.com/SteveWarm/spider/downloader
go get github.com/SteveWarm/spider/yaner
go install github.com/SteveWarm/spider/yaner
```

# 使用

```
Usage of yaner:
  -H string
    	请求中加入的header值，默认取环境变量SPIDER_HEADER的值
  -S int
    	分块大小,单位KB (default 1024)
  -db string
    	存储任务进度文件
  -load
    	是否加载已经存在的任务
  -name string
    	保存的文件名，不建议带路径
  -threads int
    	最大允许多少个线程同时下载 (default 20)
  -timeout int
    	超时时间,单位秒 (default 30)
  -url string
    	下载地址
```

## 新建下载

简单使用
执行后在当前目录下生成 v.mp4 v.mp4.cfg文件。默认20个线程下载。
```
yaner -url='http://xxxxx/v.mp4' -name='v.mp4'
```

其它参数的使用:
```
yaner -url='http://xxxxx/v.mp4' -name='v.mp4' -db='v.mp4.tmp' -threads=10 -timeout=20 -S=2048
```

## 从之前的任务中途开始

```
yaner -load -db="v.mp4.cfg"
```

还可以重新指定threads和timeout参数
```
yaner -load -db="v.mp4.cfg" -threads=10 -timeout=20
```

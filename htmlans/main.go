package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/opesun/goquery"
)

func main() {
	loadConfig()

	data, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read all from stdin faild! err:", err.Error())
		os.Exit(1)
	}

	nodes, err := goquery.ParseString(string(data))
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse html content faild! err:", err.Error())
		os.Exit(1)
	}

	find(nodes, 1)
}

func find(nodes goquery.Nodes, c int) {
	n := len(selectors)
	if c < n {
		finds := nodes.Find(selectors[c])
		if c == n-1 {
			for i := 0; i < finds.Length(); i++ {
				output(finds.Eq(i))
			}
		} else {
			for i := 0; i < finds.Length(); i++ {
				find(finds.Eq(i), c+1)
			}
		}
	}
}

func output(nodes goquery.Nodes) {
	if *attr == "" {
		fmt.Println(nodes.OuterHtml())
	} else {
		fmt.Println(nodes.Attr(*attr))
	}
}

var args []string
var selectors []string
var attr *string

func loadConfig() {
	isSelector := false
	n := len(os.Args)
	for i := 1; i < n; i++ {
		if "-s" == os.Args[i] {
			isSelector = true
			continue
		}
		if isSelector {
			selectors = append(selectors, os.Args[i])
		} else {
			args = append(args, os.Args[i])
		}
	}
	var CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	attr = CommandLine.String("attr", "", "")
	err := CommandLine.Parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		CommandLine.PrintDefaults()
		os.Exit(1)
	}
}

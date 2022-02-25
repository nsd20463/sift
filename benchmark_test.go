package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"testing"

	"github.com/svent/go-flags"
	"golang.org/x/term"
)

var init_once sync.Once
var targets []string

func BenchmarkSiftMain(b *testing.B) {
	init_once.Do(func() {
		input_args := os.Args[1:]
		// find and advance past the first "--" argument (if any)
		for i, a := range input_args {
			if a == "--" {
				input_args = input_args[i+1:]
				break
			}
		}
		if len(input_args) == len(os.Args[1:]) {
			// no --, so no arguments for sift
			input_args = nil
		}

		var args []string
		var err error

		parser := flags.NewNamedParser("sift-benchmark", flags.HelpFlag|flags.PassDoubleDash)
		parser.AddGroup("Options", "Options", &options)
		parser.Name = "sift-benchmark"
		parser.Usage = "[OPTIONS] PATTERN [FILE|PATH|tcp://HOST:PORT]...\n" +
			"  sift [OPTIONS] [-e PATTERN | -f FILE] [FILE|PATH|tcp://HOST:PORT]...\n" +
			"  sift [OPTIONS] --targets [FILE|PATH]..."

		// temporarily parse options to see if the --no-conf/--conf options were used and
		// then discard the result
		options.LoadDefaults()
		args, err = parser.ParseArgs(input_args)
		if err != nil {
			if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
				fmt.Println(e.Error())
				os.Exit(0)
			} else {
				errorLogger.Println(err)
				os.Exit(2)
			}
		}
		noConf := options.NoConfig
		configFile := options.ConfigFile
		options = Options{}

		// perform full option parsing respecting the --no-conf/--conf options
		options.LoadDefaults()
		options.LoadConfigs(noConf, configFile)
		args, err = parser.ParseArgs(input_args)
		if err != nil {
			errorLogger.Println(err)
			os.Exit(2)
		}

		for _, pattern := range options.Patterns {
			global.matchPatterns = append(global.matchPatterns, pattern)
		}

		if options.PatternFile != "" {
			f, err := os.Open(options.PatternFile)
			if err != nil {
				errorLogger.Fatalln("Cannot open pattern file:\n", err)
			}
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				pattern := scanner.Text()
				global.matchPatterns = append(global.matchPatterns, pattern)

			}
		}
		if len(global.matchPatterns) == 0 {
			if len(args) == 0 && !(options.PrintConfig || options.WriteConfig ||
				options.TargetsOnly || options.ListTypes) {
				errorLogger.Fatalln("No pattern given. Try 'sift --help' for more information.")
			}
			if len(args) > 0 && !options.TargetsOnly {
				global.matchPatterns = append(global.matchPatterns, args[0])
				args = args[1:len(args)]
			}
		}

		if len(args) == 0 {
			// check whether there is input on STDIN
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				targets = []string{"-"}
			} else {
				targets = []string{"."}
			}
		} else {
			targets = args[0:len(args)]
		}

		// expand arguments containing patterns on Windows
		if runtime.GOOS == "windows" {
			targetsExpanded := []string{}
			for _, t := range targets {
				if t == "-" {
					targetsExpanded = append(targetsExpanded, t)
					continue
				}
				expanded, err := filepath.Glob(t)
				if err == filepath.ErrBadPattern {
					errorLogger.Fatalf("cannot parse argument '%s': %s\n", t, err)
				}
				if expanded != nil {
					for _, e := range expanded {
						targetsExpanded = append(targetsExpanded, e)
					}
				}
			}
			targets = targetsExpanded
		}

		if err := options.Apply(global.matchPatterns, targets); err != nil {
			errorLogger.Fatalf("cannot process options: %s\n", err)
		}

		global.matchRegexes = make([]*regexp.Regexp, len(global.matchPatterns))
		for i := range global.matchPatterns {
			global.matchRegexes[i], err = regexp.Compile(global.matchPatterns[i])
			if err != nil {
				errorLogger.Fatalf("cannot parse pattern: %s\n", err)
			}
		}

		b.ResetTimer()
	})

	for i := 0; i < b.N; i++ {
		_, err := executeSearch(targets)
		if err != nil {
			errorLogger.Println(err)
		}
	}
}

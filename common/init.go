package common

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

var (
	Port         = flag.Int("port", 3000, "the listening port")
	PrintVersion = flag.Bool("version", false, "print version and exit")
	PrintHelp    = flag.Bool("help", false, "print help and exit")
	LogDir       = flag.String("log-dir", "./logs", "specify the log directory")
)

func printHelp() {
	fmt.Println("Happy API " + Version + " - All in Happy API service for OpenAI API.")
	fmt.Println("Copyright (C) 2023 JustSong. All rights reserved.")
	fmt.Println("GitHub: https://github.com/songquanpeng/one-api")
	fmt.Println("Usage: one-api [--port <port>] [--log-dir <log directory>] [--version] [--help]")
}

func init() {
	// 加载.env文件
	err := godotenv.Load()
	if err != nil {
		SysLog("failed to load .env file: " + err.Error())
	}
	flag.Parse()

	if *PrintVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	if *PrintHelp {
		printHelp()
		os.Exit(0)
	}

	if os.Getenv("SESSION_SECRET") != "" {
		if os.Getenv("SESSION_SECRET") == "random_string" {
			SysError("SESSION_SECRET is set to an example value, please change it to a random string.")
		} else {
			SessionSecret = os.Getenv("SESSION_SECRET")
		}
	}
	if os.Getenv("SQLITE_PATH") != "" {
		SQLitePath = os.Getenv("SQLITE_PATH")
	}
	if *LogDir != "" {
		var err error
		*LogDir, err = filepath.Abs(*LogDir)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := os.Stat(*LogDir); os.IsNotExist(err) {
			err = os.Mkdir(*LogDir, 0777)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

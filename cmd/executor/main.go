package main

import (
	"flag"
	"log"

	"exchange/internal/runtime"
)

func main() {
	opts := runtime.DefaultOptions("executor")
	flag.StringVar(&opts.FrontendMode, "frontend", opts.FrontendMode, "frontend mode: dev, build or off")
	flag.StringVar(&opts.FrontendDir, "frontend-dir", opts.FrontendDir, "frontend working directory")
	flag.StringVar(&opts.FrontendHost, "frontend-host", opts.FrontendHost, "frontend dev server host")
	flag.StringVar(&opts.FrontendPort, "frontend-port", opts.FrontendPort, "frontend dev server port")
	flag.StringVar(&opts.FrontendPackageManager, "frontend-package-manager", opts.FrontendPackageManager, "frontend package manager executable")
	flag.Parse()

	if err := runtime.RunAllWithOptions("executor", opts); err != nil {
		log.Fatal(err)
	}
}

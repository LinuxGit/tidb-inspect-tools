package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/ngaut/log"
)

var (
	logFile          = flag.String("log-file", "", "log file path")
	logLevel         = flag.String("log-level", "info", "log level: debug, info, warn, error, fatal")
	logRotate        = flag.String("log-rotate", "day", "log file rotate type: hour/day")
	configFile       = flag.String("config", "", "path to configuration file")
	clusterName      = flag.String("cluster-name", "", "TiDB Cluster name")
	smtpSmarthost    = flag.String("smtp_smarthost", "", "SMTP Server")
	smtpAuthUsername = flag.String("smtp_auth_username", "", "SMTP Auth Username")
	smtpAuthPassword = flag.String("smtp_auth_password", "", "SMTP Auth Password")
	smtpFrom         = flag.String("smtp_from", "", "SMTP From")
	smtpTo           = flag.String("smtp_to", "", "SMTP To")
)

func main() {
	flag.Parse()

	if *clusterName == "" {
		log.Fatalf("missing parameter: -cluster-name")
	}

	err := SetConfig(*configFile)
	if err != nil {
		log.Fatalf("parsing configure file error: %v", err)
	}

	log.SetLevelByString(*logLevel)
	if *logFile != "" {
		log.SetOutputByName(*logFile)
		if *logRotate == "hour" {
			log.SetRotateByHour()
		} else {
			log.SetRotateByDay()
		}
	}

	r := &Run{}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		syscall.SIGKILL,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	go func() {
		sig := <-sc
		log.Infof("got signal [%d] to exit", sig)
		os.Exit(0)
	}()

	r.Scheduler()
}

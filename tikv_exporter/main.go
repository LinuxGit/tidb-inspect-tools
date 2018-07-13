package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/ngaut/log"
	"github.com/pingcap/kvproto/pkg/debugpb"
	"github.com/pingcap/tidb-inspect-tools/pkg/utils"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"google.golang.org/grpc"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	stores = []string{}
)

type tikvOpts struct {
	addrs string
}

// ParseHostPortAddr returns a host:port list
func ParseHostPortAddr(s string) ([]string, error) {
	strs := strings.Split(s, ",")
	addrs := make([]string, 0, len(strs))

	for _, str := range strs {
		str = strings.TrimSpace(str)

		_, _, err := net.SplitHostPort(str)
		if err != nil {
			return nil, errors.Annotatef(err, `tikv.addrs does not have the form "host:port": %s`, str)
		}

		addrs = append(addrs, str)
	}

	return addrs, nil
}

func checkParameters(opts tikvOpts) {
	if opts.addrs == "" {
		log.Fatalf("missing startup parameter: --tikv.addrs")
	}
}

func sanitizeLabels(
	metricFamilies map[string]*dto.MetricFamily,
	groupingLabels map[string]string,
) {
	for _, mf := range metricFamilies {
		for _, m := range mf.GetMetric() {
			for key, value := range groupingLabels {
				l := &dto.LabelPair{
					Name:  proto.String(key),
					Value: proto.String(value),
				}
				m.Label = append(m.Label, l)
			}
			sort.Sort(prometheus.LabelPairSorter(m.Label))
		}
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	allMetrics := make([]*dto.MetricFamily, 0, 1024)
	for _, store := range stores {
		fmt.Printf("%s\n", store)

		tikvConn, err := grpc.Dial(store, grpc.WithInsecure())
		if err != nil {
			log.Fatal(err)
		}

		tikvClient := debugpb.NewDebugClient(tikvConn)
		metrics, err := tikvClient.GetMetrics(ctx, &debugpb.GetMetricsRequest{})
		if err != nil {
			log.Fatal(err)
		}

		mData := metrics.GetPrometheus()
		storeID := metrics.GetStoreId()
		fmt.Println(storeID)

		labels := map[string]string{
			"job":      fmt.Sprintf("tikv_%d", storeID),
			"instance": store,
		}

		var parser expfmt.TextParser
		metricFamilies, err := parser.TextToMetricFamilies(bytes.NewBufferString(mData))
		if err != nil {
			log.Fatal(err)
		}

		sanitizeLabels(metricFamilies, labels)

		for _, m := range metricFamilies {
			allMetrics = append(allMetrics, m)
		}

		tikvConn.Close()
	}

	for _, m := range allMetrics {
		var b bytes.Buffer
		expfmt.MetricFamilyToText(&b, m)
		fmt.Fprintf(w, "%s", b.String())
	}
}

func main() {
	var (
		listenAddress = kingpin.Flag("web.listen-address", "Address on which to expose metrics and web interface.").Default(":9600").String()
		metricsPath   = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
		logFile       = kingpin.Flag("log-file", "Log file path.").Default("").String()
		logLevel      = kingpin.Flag("log-level", "Log level: debug, info, warn, error, fatal.").Default("info").String()
		logRotate     = kingpin.Flag("log-rotate", "Log file rotate type: hour/day.").Default("day").String()

		opts = tikvOpts{}
	)

	kingpin.Flag("tikv.addrs", "Addresses (host:port) of TiKV server nodes, comma separated.").Default("").StringVar(&opts.addrs)
	kingpin.Version(utils.GetRawInfo("tikv_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	checkParameters(opts)

	log.SetLevelByString(*logLevel)
	if *logFile != "" {
		log.SetOutputByName(*logFile)
		if *logRotate == "hour" {
			log.SetRotateByHour()
		} else {
			log.SetRotateByDay()
		}
	}

	log.Info("Starting tikv_exporter")

	stores, _ = ParseHostPortAddr(opts.addrs)
	// if err != nil {
	//     log.Fatalf("initialize tikv_exporter error, %v", errors.ErrorStack(err))
	// }

	http.HandleFunc(*metricsPath, handler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
	        <head><title>TiKV Exporter</title></head>
	        <body>
	        <h1>TiKV Exporter</h1>
	        <p><a href='` + *metricsPath + `'>Metrics</a></p>
	        </body>
	        </html>`))
	})

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

	log.Info("Listening on", *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}

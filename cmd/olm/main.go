package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/client"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/controller/install"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/controller/operators/olm"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/operatorclient"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/signals"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/metrics"
	olmversion "github.com/operator-framework/operator-lifecycle-manager/pkg/version"
)

const (
	defaultWakeupInterval = 5 * time.Minute
)

// config flags defined globally so that they appear on the test binary as well
var (
	kubeConfigPath = flag.String(
		"kubeconfig", "", "absolute path to the kubeconfig file")

	wakeupInterval = flag.Duration(
		"interval", defaultWakeupInterval, "wake up interval")

	watchedNamespaces = flag.String(
		"watchedNamespaces", "", "comma separated list of namespaces for alm operator to watch. "+
			"If not set, or set to the empty string (e.g. `-watchedNamespaces=\"\"`), "+
			"olm operator will watch all namespaces in the cluster.")

	debug = flag.Bool(
		"debug", false, "use debug log level")

	version = flag.Bool("version", false, "displays olm version")
)

func init() {
	metrics.Register()
}

// main function - entrypoint to ALM operator
func main() {
	stopCh := signals.SetupSignalHandler()

	// Parse the command-line flags.
	flag.Parse()

	// Check if version flag was set
	if *version {
		fmt.Print(olmversion.String())

		// Exit early
		os.Exit(0)
	}

	// Set log level to debug if `debug` flag set
	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	// `namespaces` will always contain at least one entry: if `*watchedNamespaces` is
	// the empty string, the resulting array will be `[]string{""}`.
	namespaces := strings.Split(*watchedNamespaces, ",")
	for _, ns := range namespaces {
		if ns == v1.NamespaceAll {
			namespaces = []string{v1.NamespaceAll}
			break
		}
	}

	// Create a client for OLM
	crClient, err := client.NewClient(*kubeConfigPath)
	if err != nil {
		log.Fatalf("error configuring client: %s", err.Error())
	}

	logger := log.New()

	opClient := operatorclient.NewClientFromConfig(*kubeConfigPath, logger)

	// Create a new instance of the operator.
	operator, err := olm.NewOperator(logger, crClient, opClient, &install.StrategyResolver{}, *wakeupInterval, namespaces)

	if err != nil {
		log.Fatalf("error configuring operator: %s", err.Error())
	}

	// Serve a health check.
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// TODO: both of the following require vendor updates (add k8s.io/apiserver and update prometheus)
	//healthz.InstallHandler(mux) //(less code)
	//mux.Handle("/metrics", promhttp.Handler()) //other form is deprecated
	http.Handle("/metrics", prometheus.Handler())
	go http.ListenAndServe(":8080", nil)

	_, done := operator.Run(stopCh)
	<-done
}

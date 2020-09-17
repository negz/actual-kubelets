package main

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"
	cli "github.com/virtual-kubelet/node-cli"
	logruscli "github.com/virtual-kubelet/node-cli/logrus"
	"github.com/virtual-kubelet/node-cli/opts"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"

	"github.com/negz/actual-kubelets/internal/kubernetes"
)

var (
	name            = "actual-kubelets"
	buildVersion    = "N/A"
	buildTime       = "N/A"
	k8sVersion      = "v1.18.5" // This should follow the version of k8s.io/kubernetes we are importing
	numberOfWorkers = 50
)

func main() {
	ctx := cli.ContextWithCancelOnSignal(context.Background())

	logger := logrus.StandardLogger()
	log.L = logruslogger.FromLogrus(logrus.NewEntry(logger))
	logConfig := &logruscli.Config{LogLevel: "info"}

	o, err := opts.FromEnv()
	if err != nil {
		log.G(ctx).Fatal(err)
	}
	o.Provider = name
	o.Version = strings.Join([]string{k8sVersion, name, buildVersion}, "-")
	o.PodSyncWorkers = numberOfWorkers

	node, err := cli.New(ctx,
		cli.WithBaseOpts(o),
		cli.WithCLIVersion(buildVersion, buildTime),
		cli.WithProvider(name, kubernetes.NewProvider),
		cli.WithPersistentFlags(logConfig.FlagSet()),
		cli.WithPersistentPreRunCallback(func() error {
			return logruscli.Configure(logConfig, logger)
		}),
	)

	if err != nil {
		log.G(ctx).Fatal(err)
	}

	if err := node.Run(ctx); err != nil {
		log.G(ctx).Fatal(err)
	}
}

package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	_ "github.com/dimiro1/banner/autoload"
	"github.com/kelseyhightower/envconfig"
	"os"
	"text/tabwriter"
	"time"
)

const (
	// configTemplate used to display the configuration values for informational/debbug purposes
	configTemplate = `This service is configured by the environment. The following
are the configuration values:
    KEY	VALUE	DESCRIPTION
    {{range .}}{{usage_key .}}	{{.Field}}	{{usage_description .}}
    {{end}}`
)

type Application struct {
	OnosConnectUrl     string `default:"http://karaf:karaf@127.0.0.1:8181" envconfig:"ONOS_CONNECT_URL" desc:"URL with which to connect to ONOS"`
	OvsDpid            string `default:"of:00000800276f723f" envconfig:"OVS_DPID" desc:"DPID of switch to manager"`
	CreateFlowTemplate string `default:"/var/templates/create.tmpl" envconfig:"CREATE_FLOW_TEMPLATE" desc:"Template file used to create flow rule in ONOS"`
	Interval	   time.Duration `default:"30s" envconfig:"INTERVAL" desc:"Frequency to check for correct flows"`
	LogLevel           string `default:"warning" envconfig:"LOG_LEVEL" desc:"detail level for logging"`
	LogFormat          string `default:"text" envconfig:"LOG_FORMAT" desc:"log output format, text or json"`
}

var log = logrus.New()

func main() {

	app := Application{}
	err := envconfig.Process("LETMEIN", &app)
	if err != nil {
		log.Fatalf("Unable to parse configuration options : %s", err)
	}

	tabs := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	err = envconfig.Usagef("", &app, tabs, configTemplate)
	if err != nil {
		panic(err)
	}
	tabs.Flush()
	fmt.Println()

	// Establish logging configuraton
	switch app.LogFormat {
	case "json":
		log.Formatter = &logrus.JSONFormatter{}
	default:
		log.Formatter = &logrus.TextFormatter{
			FullTimestamp: true,
			ForceColors:   true,
		}
	}
	level, err := logrus.ParseLevel(app.LogLevel)
	if err != nil {
		log.Errorf("Invalid error level specified: '%s', defaulting to WARN level", app.LogLevel)
		level = logrus.WarnLevel
	}
	log.Level = level

	log.Info("Starting OVS Extra Flow Manager (letmein)")

	/*
	 * Synchronization should be triggered by events in ONOS, but as we can't get events from
	 * ONOS we use a polling loop.
	 */
	for {
		log.Infof("Synchronize required S-TAG VIDs from ONOS to OVS switch %s", app.OvsDpid)
		app.Synchronize()
                log.Info("COMPLETE")
		time.Sleep(app.Interval)
	}
}

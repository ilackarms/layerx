package main

import (
	"flag"
	"fmt"
	"net"

	"github.com/Sirupsen/logrus"
	"github.com/emc-advanced-dev/layerx/layerx-core/layerx_rpi_client"
	"github.com/emc-advanced-dev/layerx/layerx-mesos-rpi/layerx_rpi_api"
	"github.com/emc-advanced-dev/layerx/layerx-mesos-rpi/mesos_framework_api"
	"github.com/emc-advanced-dev/pkg/errors"
	"github.com/gogo/protobuf/proto"
	"github.com/layer-x/layerx-commons/lxmartini"
	"github.com/layer-x/layerx-commons/lxutils"
	"github.com/mesos/mesos-go/mesosproto"
	"github.com/mesos/mesos-go/scheduler"
	"github.com/emc-advanced-dev/pkg/logger"
)

const rpi_name = "Mesos-RPI-0.0.0"

func main() {
	port := flag.Int("port", 4000, "listening port for mesos rpi")
	master := flag.String("master", "127.0.0.1:5050", "url of mesos master")
	debug := flag.Bool("debug", false, "turn on debugging, default: false")
	layerX := flag.String("layerx", "", "layer-x url, e.g. \"10.141.141.10:3000\"")
	localIpStr := flag.String("localip", "", "binding address for the rpi")
	name := flag.String("name", rpi_name, "name to use to register to layerx")
	user := flag.String("user", "root", "mesos user to use on mesos")
	flag.Parse()

	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debugf("debugging activated")
	}
	logrus.AddHook(&logger.LoggerNameHook{*name})

	localip := net.ParseIP(*localIpStr)
	if localip == nil {
		var err error
		localip, err = lxutils.GetLocalIp()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Fatalf("retrieving local ip")
		}
	}

	rpiFramework := prepareFrameworkInfo(*layerX, *user)
	rpiClient := &layerx_rpi_client.LayerXRpi{
		CoreURL: *layerX,
		RpiName: *name,
	}

	logrus.WithFields(logrus.Fields{
		"rpi_url": fmt.Sprintf("%s:%v", localip.String(), *port),
	}).Infof("registering to layerx")

	err := rpiClient.RegisterRpi(*name, fmt.Sprintf("%s:%v", localip.String(), *port))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error":      err.Error(),
			"layerx_url": *layerX,
		}).Errorf("registering to layerx")
	}

	rpiScheduler := mesos_framework_api.NewRpiMesosScheduler(rpiClient)

	config := scheduler.DriverConfig{
		Scheduler:  rpiScheduler,
		Framework:  rpiFramework,
		Master:     *master,
		Credential: (*mesosproto.Credential)(nil),
	}

	go func() {
		driver, err := scheduler.NewMesosSchedulerDriver(config)
		if err != nil {
			err = errors.New("initializing mesos schedulerdriver", err)
			logrus.WithFields(logrus.Fields{
				"error":     err,
				"mesos_url": *master,
			}).Fatalf("error initializing mesos schedulerdriver")
		}
		status, err := driver.Run()
		if err != nil {
			err = errors.New("Framework stopped with status "+status.String(), err)
			logrus.WithFields(logrus.Fields{
				"error":     err,
				"mesos_url": *master,
			}).Fatalf("error running mesos schedulerdriver")
			return
		}
	}()
	mesosSchedulerDriver := rpiScheduler.GetDriver()
	rpiServerWrapper := layerx_rpi_api.NewRpiApiServerWrapper(rpiClient, *master, rpiScheduler.TaskChan, mesosSchedulerDriver)
	errc := make(chan error)
	m := rpiServerWrapper.WrapWithRpi(lxmartini.QuietMartini(), errc)
	go m.RunOnAddr(fmt.Sprintf(":%v", *port))

	logrus.WithFields(logrus.Fields{
		"config": config,
	}).Infof("Layer-X Mesos RPI Initialized...")

	for {
		err = <-errc
		if err != nil {
			logrus.WithFields(logrus.Fields{"error": err}).Errorf("LayerX Mesos RPI Failed!")
		}
	}
}

func prepareFrameworkInfo(layerxUrl, user string) *mesosproto.FrameworkInfo {
	return &mesosproto.FrameworkInfo{
		User: proto.String(user),
		//		Id: &mesosproto.FrameworkID{
		//			Value: proto.String("lx_mesos_rpi_framework_3"),
		//		},
		FailoverTimeout: proto.Float64(0),
		Name:            proto.String("Layer-X Mesos RPI Framework"),
		WebuiUrl:        proto.String(layerxUrl),
	}
}

package main

import (
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"gopkg.in/yaml.v2"
)

//
// Config file format
//
type Config struct {
	// Global debug setting
	Debug bool `yaml:"debug"`

	//
	// MQTT Brokers that we use for everything below.  Every topic in the config file
	// needs to be in the form of BROKER_NAME:TOPIC, with BROKER_NAME being a broker
	// configured in this section.
	//
	Brokers []BrokerConfig `yaml:"brokers"`

	//
	// Client, which posts uptime and listens for commands.  It also needs a better name.
	//
	DeviceMgmt DeviceMgmtConfig `yaml:"device"`

	//
	// SDR to MQTT.  I suppose this could also be an array some
	// day when I connect multiple SDRs to a single Pi.
	//
	SDR SDRConfig `yaml:"sdr"`

	//
	// Output to LED matrix, which pulls from many sources (including MQTT)
	//
	Matrix struct {
		Weather      WeatherConfig      `yaml:"weather"`
		TempSensors  []TempSensorConfig `yaml:"sensors"`
		RemoteImages RemoteImageConfig  `yaml:"remote"`
		LocalImages  LocalImagesConfig  `yaml:"local"`
		Strings      StringsConfig      `yaml:"strings"`
		Mirror       []MirrorConfig     `yaml:"mirror"`
	} `yaml:"matrix"`
}

func handleError(err error) {
	println(err.Error())
	usage()
	os.Exit(-1)
}

func usage() {
	println("Usage: " + os.Args[0] + " [CFGFILEPATH]")
}

//
// Main
//
func main() {
	var cfg Config

	// Read in the config file
	cfgPath := "config.yml"
	if len(os.Args) >= 2 {
		cfgPath = os.Args[1]
	}

	f, err := os.Open(cfgPath)
	if err != nil {
		handleError(err)
	}
	defer f.Close()
	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&cfg)
	if err != nil {
		handleError(err)
	}

	if cfg.Debug == true {
		log.Infof("DEBUG")
		log.SetLevel(log.DebugLevel)
	}

	// Fire up the brokerMux
	bmux := newBrokerMux(cfg.Brokers)

	// Fire up the SDR code
	initSDR(bmux, &cfg.SDR)

	// Fire up some data sources
	initRemoteImages(bmux, cfg.Matrix.RemoteImages)
	initTempSensors(bmux, cfg.Matrix.TempSensors)
	initWeather(bmux, cfg.Matrix.Weather)
	initLocalImages(bmux, cfg.Matrix.LocalImages)
	initStrings(bmux, cfg.Matrix.Strings)
	initMirror(bmux, cfg.Matrix.Mirror)

	// Run the device management code
	runDeviceMgmt(bmux, cfg.DeviceMgmt)

	// Sit and spin
	for true {
		time.Sleep(time.Minute)
	}
}

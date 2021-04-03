package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)

// SDRConfig contains all of the stuff you can configure for the SDR
type SDRConfig struct {
	// Publishing
	Topic    string `yaml:"topic"`
	Interval int    `yaml:"interval"`
	Allow    []struct {
		Model string `yaml:"model"`
		ID    int    `yaml:"id"`
	} `yaml:"allow"`

	// RTL_433
	RTL433 struct {
		App       string `yaml:"app"`
		Protocols []int  `yaml:"protocols"`
		OnTime    int    `yaml:"onSeconds"`
		OffTime   int    `yaml:"offSeconds"`
		Deadman   int    `yaml:"deadman"`
	} `yaml:"rtl_433"`
}

// Incoming raw temp data from rtl_433
type weatherSensorSDR struct {
	Model string `json:"model"`
	ID    int    `json:"id"`

	TemperatureC float32 `json:"temperature_C,omitempty"`
	TemperatureF float32 `json:"temperature_F,omitempty"`

	Humidity  float32 `json:"humidity,omitempty"`
	WindSpeed float32 `json:"wind_avg_km_h,omitempty"`
	WindDir   float32 `json:"wind_dir_deg,omitempty"`
	Rain      float32 `json:"rain_mm,omitempty"`
}

// Outgoing data to MQTT
type weatherSensorDataMQTT struct {
	dirty bool
	id    int

	Temperature float32 `json:"temperature"`
	Humidity    float32 `json:"humidity,omitempty"`
	WindSpeed   float32 `json:"windSpeed,omitempty"`
	WindDir     float32 `json:"windDir,omitempty"`
	Rain        float32 `json:"rain,omitempty"`
}

// sdrData just wraps all of the data.  Fancy.
type sdrData struct {
	// Config read from yml land
	sdrCfg *SDRConfig

	// MQTT client we fire up based on the config
	bmux BrokerMux

	// Map of sensors to allow, if provided.  If this is empty, all are alloed
	allowMap map[string]bool

	// Map of a sensorHash to the last data we received for said sensor
	sensors map[string]*weatherSensorDataMQTT

	// Channel for accepting incoming sensor data from rtl_433
	dataChan chan []byte
}

func initSDR(bmux BrokerMux, cfg *SDRConfig) {
	//
	// Init the app
	//
	data := &sdrData{
		sdrCfg:   cfg,
		bmux:     bmux,
		sensors:  make(map[string]*weatherSensorDataMQTT),
		allowMap: make(map[string]bool),
		dataChan: make(chan []byte),
	}

	// See if we even have a topic
	if len(cfg.Topic) == 0 {
		return
	}

	// Fill in the allow map
	for _, allow := range cfg.Allow {
		data.addAllowedSensor(allow.Model, allow.ID)
	}

	// Fire up the goroutine that manages rtl_433
	go data.runRTL433()

	// Spin forever, emitting processed and rate limited data.  The RTL433
	// goroutine sends us data via dataChan, so all of the locking is
	// magically taken for of for us.  Way to be lazy!
	go func() {
		// Fire up the emit ticker
		interval := cfg.Interval
		if interval < 1 {
			interval = 1
		}
		ticker := time.NewTicker(time.Duration(interval) * time.Minute)

		// Loop forever on incoming data and timed publishing
		for {
			select {
			case d := <-data.dataChan:
				data.consume(d)
			case <-ticker.C:
				data.emit()
			}
		}
	}()
}

//
// runSDR is a goroutine that fires off rtl_433 and sends the data via a channel
//
func (sdr *sdrData) runRTL433() {
	// Form the args
	args := []string{"-F", "json", "-C", "si"}

	if sdr.sdrCfg.RTL433.OnTime > 0 {
		args = append(args, "-T")
		args = append(args, strconv.Itoa(sdr.sdrCfg.RTL433.OnTime))
	}

	for _, protocol := range sdr.sdrCfg.RTL433.Protocols {
		args = append(args, "-R")
		args = append(args, strconv.Itoa(protocol))
	}

	log.Infof("sdr: args: %v", args)

	idleLoops := 0
	active := false

	// Loop a bit
	for {
		// Prep the command
		app := "rtl_433"
		if sdr.sdrCfg.RTL433.App != "" {
			app = sdr.sdrCfg.RTL433.App
		}
		cmd := exec.Command(app, args...)
		stdout, _ := cmd.StdoutPipe()
		done := make(chan struct{})
		scanner := bufio.NewScanner(stdout)

		// Yet another goroutine to handle the data ...
		idle := active
		go func() {
			log.Debugf("sdr: loop: start")
			for scanner.Scan() {
				idle = false
				active = true
				sdr.dataChan <- []byte(scanner.Text())
			}
			log.Debugf("sdr: loop: end")
			done <- struct{}{}
		}()

		// ... while I just sit here
		if err := cmd.Start(); err != nil {
			log.Errorf("sdr: start: %v", err)
			return
		}

		// FIXME: Tight failure loop detection?
		<-done

		// Attempt to reboot once if the deadman goes off
		if sdr.sdrCfg.RTL433.Deadman > 0 {
			if idle {
				idleLoops = idleLoops + 1
				if idleLoops >= sdr.sdrCfg.RTL433.Deadman {
					sdr.sdrCfg.RTL433.Deadman = 0
					log.Info("sdr: reboot due to deadman")
					exec.Command("/bin/bash", "-c", "/usr/bin/sudo /usr/sbin/reboot").Output()
				}
			} else {
				idleLoops = 0
			}
		}

		// Sleep a bit. Duty cycle and all
		if sdr.sdrCfg.RTL433.OffTime > 0 {
			time.Sleep(time.Duration(sdr.sdrCfg.RTL433.OffTime) * time.Second)
		}
	}
}

//
// consume gets called on the main thread whenever we have new raw data from rtl_433.  This is supposed
// to be JSON in a format we know, but trust no one.
//
func (sdr *sdrData) consume(payload []byte) {
	var data weatherSensorSDR

	// Parse the JSON
	if err := json.Unmarshal(payload, &data); err != nil {
		log.Infof("sdr: consume: %v: %v", err, data)
	}

	// See if we even care
	if sdr.sensorAllowed(data) == false {
		return
	}

	log.Debugf("sdr: consume: %v", data)

	// Create a hash.  This doesn't need to be that fancy given the low sensor count, and even
	// using the ID should be good enough since we're already in a world of hurt if these overlap.
	hash := fmt.Sprintf("%s:%d", data.Model, data.ID)

	// Create the sensor if it is missing
	sensor, ok := sdr.sensors[hash]
	if ok != true {
		sensor = &weatherSensorDataMQTT{id: data.ID, dirty: false}
		sdr.sensors[hash] = sensor
	}

	// Clear before adding new data
	if sensor.dirty == false {
		sensor.Temperature = 0.0
		sensor.Humidity = 0.0
		sensor.WindSpeed = 0.0
		sensor.WindDir = 0.0
		sensor.Rain = 0.0
		sensor.dirty = true
	}

	//
	// There has to be an easier way to do this...
	//
	if data.TemperatureC != 0 {
		sensor.Temperature = data.TemperatureC
	}
	if data.TemperatureF != 0 {
		sensor.Temperature = (data.TemperatureF - 32.0) * (5.0 / 9.0)
	}
	if data.Humidity != 0 {
		sensor.Humidity = data.Humidity
	}
	if data.WindSpeed != 0 && data.WindDir != 0 {
		sensor.WindSpeed = data.WindSpeed
		sensor.WindDir = data.WindDir
	}
	if data.Rain != 0 {
		sensor.Rain = data.Rain
	}
}

//
// emit gets called on the main goroutine when it is time to event data we have been
// accumulating
//
func (sdr *sdrData) emit() bool {

	for _, sensor := range sdr.sensors {

		if sensor.dirty {
			sensor.dirty = false

			out, err := json.Marshal(sensor)
			if err != nil {
				log.Errorf("sdr: emit: %v", err)
				continue
			}

			topic := sdr.sdrCfg.Topic + strconv.Itoa(sensor.id)
			log.Debugf("sdr: emit: %s: %s", topic, out)

			// Fire off another goroutine to send since this is blocking consume()
			go func() {
				token := sdr.bmux.Publish(topic, 0, false, out)
				token.Wait()
			}()
		}
	}

	return false
}

//
// Allow lists of sensors to allow.  Allow.
//
func createAllowFilterHash(model string, id int) string {
	return fmt.Sprintf("%s:%d", model, id)
}

func (sdr *sdrData) addAllowedSensor(model string, id int) {
	hash := createAllowFilterHash(model, id)
	sdr.allowMap[hash] = true
}

func (sdr *sdrData) sensorAllowed(sdrJSON weatherSensorSDR) bool {
	if len(sdr.allowMap) == 0 {
		return true
	}
	_, ok := sdr.allowMap[createAllowFilterHash(sdrJSON.Model, sdrJSON.ID)]
	return ok
}

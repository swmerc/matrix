package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

// TempSensorConfig supports reading in sensor data from a bunch of topics (the "subs") and
// posting a single string to another topic at the given offsets.
type TempSensorConfig struct {
	Topic   string       `yaml:"topic"`
	Jobs    JobRunnerCfg `yaml:"jobs"`
	Sensors []struct {
		Sub  string `yaml:"sub"`
		Name string `yaml:"name"`
	}
}

//
// Actual gory details
//
type tempSensorsImpl struct {
	// How/when to report.  Probably should grab a pointer instead of a copy.
	config []TempSensorConfig

	// MQTT client to use to report
	bmux BrokerMux

	// Actual temp sensor data
	tempSensors map[string]*tempSensor

	// Channel for incoming temperature infomation
	tempChan chan mqtt.Message

	// Channel for kicking off a group report
	runChan chan int
}

//
// Init
//
func initTempSensors(bmux BrokerMux, cfgs []TempSensorConfig) {

	// Create the basic struct
	impl := &tempSensorsImpl{
		bmux:        bmux,
		config:      cfgs,
		tempChan:    make(chan mqtt.Message),
		runChan:     make(chan int),
		tempSensors: make(map[string]*tempSensor),
	}

	// Create an array of pointers to job runners we can use to start all of them after we start
	// our main goroutine
	runners := make([]JobRunner, 0, 16)

	// Walk the sensorGroups and make actual sensors and jobs for everything so we can track changes.
	for groupIdx, group := range cfgs {
		log.Debugf("tempSensors: init group: %s:%d", group.Topic, groupIdx)

		// Create the runner, creating a copy of the groupIdx for the closure
		idx := groupIdx
		runner := NewJobRunner("sensors-"+strconv.Itoa(groupIdx), group.Jobs, func() {
			impl.runChan <- idx
		})
		runners = append(runners, runner)

		// Add tracking for the actual sensors we want to monitor, and subscribe to each.
		// this is a bit of a pain compared to subscribing to "temperature/blah", but I
		// think it is better overall.
		for _, sensorCfg := range group.Sensors {
			log.Debugf("tempSensors: init sensor: %s (%s)", sensorCfg.Sub, sensorCfg.Name)
			sensor := &tempSensor{
				name:  sensorCfg.Name,
				dirty: false,
			}

			t := bmux.Subscribe(sensorCfg.Sub, 0, func(client mqtt.Client, msg mqtt.Message) {
				impl.HandleTempMessage(msg)
			})
			t.Wait()
			if t.Error() != nil {
				log.Panicf("tempSensors: %s", t.Error().Error())
			}
			impl.tempSensors[sensorCfg.Sub] = sensor
		}
	}

	// Start our goroutine
	impl.Run()

	// Start all of the job runners
	for _, r := range runners {
		r.Run()
	}
}

//
// Actual JSON from the sensors that is evented over MQTT.  I should share this with the stuff in sdr.go since
// that is the code that generates it.  Some day.
//
type tempSensorData struct {
	Temperature float32 `json:"temperature"`
	Humidity    float32 `json:"humidity"`
}

//
// temperature sensor state
//
type tempSensor struct {
	name         string
	lastTemp     float32
	lastHumidity float32
	dirty        bool
}

// Parse a message and mark as dirty
func (s *tempSensor) processMessage(msg []byte) {
	var cooked tempSensorData

	if err := json.Unmarshal(msg, &cooked); err == nil {
		s.dirty = true
		s.lastTemp = cooked.Temperature
		s.lastHumidity = cooked.Humidity
	} else {
		log.Errorf("sensors: json error: %s", err.Error())
	}
}

func (s *tempSensor) isDirty() bool {
	wasDirty := s.dirty
	s.dirty = false
	return wasDirty
}

func (s *tempSensor) getTemp() float32 {
	return s.lastTemp
}

func (s *tempSensor) getHumidity() float32 {
	return s.lastHumidity
}

//
// Everything from here down is in a goroutine  Since all data gets passed in via a channel
// we don't need to lock anything.
//
func (d *tempSensorsImpl) Run() {
	go d.loop()
}

func (d *tempSensorsImpl) loop() {
	// Just hang here waiting for temp messages or offsetJobRunner events
	for {
		select {
		case msg := <-d.tempChan:
			d.processTemp(msg.Topic(), msg.Payload())
		case idx := <-d.runChan:
			d.processGroup(idx)
		}
	}
}

func (d *tempSensorsImpl) HandleTempMessage(msg mqtt.Message) {
	d.tempChan <- msg
}

func (d *tempSensorsImpl) processTemp(topic string, payload []byte) {
	// Look up the topic to see if we care
	if sensor, ok := d.tempSensors[topic]; ok == true {
		var data tempSensorData

		// Parse the JSON and stash the latest reading
		if err := json.Unmarshal(payload, &data); err == nil {
			sensor.dirty = true
			sensor.lastTemp = (data.Temperature*9)/5 + 32
			sensor.lastHumidity = data.Humidity
			log.Debugf("sensors: processTemp: %s %.2f %.0f", topic, sensor.lastTemp, sensor.lastHumidity+0.5)
		} else {
			log.Errorf("sensors: processTemp: error=%s", err.Error())
		}
	}
}

func (d *tempSensorsImpl) processGroup(index int) {
	log.Debugf("sensors: processGroup: %d", index)

	if index < 0 || index >= len(d.config) {
		log.Panicf("bad tempSensorGroup index: %d", index)
	}

	group := &d.config[index]
	pubTopic := group.Topic
	event := ""

	// Walk the sensors in the group and create a message with all of the dirty ones
	for _, desc := range group.Sensors {
		if sensor, ok := d.tempSensors[desc.Sub]; ok == true {
			if sensor.isDirty() {
				if len(event) > 0 {
					event = event + ":"
				}
				event = event + fmt.Sprintf("%s is %.1f\xB0", sensor.name, sensor.lastTemp)
				if sensor.lastHumidity > 0 {
					event = event + fmt.Sprintf(" / %.0f%%", sensor.lastHumidity+0.5)
				}
			}
		}
	}

	if len(event) > 0 {
		log.Infof("sensors: event: %s", event)
		go d.bmux.Publish(pubTopic, 0, false, event)
	}
}

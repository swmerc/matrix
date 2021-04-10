package main

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

// MirrorConfig supportd mirroring any topic on any broker to any other topic.  Wildcards
// work on the sub side, but not on the pub side, of course.
type MirrorConfig struct {
	Sub string `yaml:"sub"`
	Pub string `yaml:"pub"`
}

func initMirror(bmux BrokerMux, cfg []MirrorConfig) {
	for _, m := range cfg {
		m := m
		log.Infof("mirror: %s -> %s", m.Sub, m.Pub)
		t := bmux.Subscribe(m.Sub, 0, func(client mqtt.Client, msg mqtt.Message) {
			log.Debugf("mirror: processing %s", m.Sub)
			go bmux.Publish(m.Pub, 0, false, msg.Payload())
		})
		t.Wait()
	}
}

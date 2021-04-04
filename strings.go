package main

import (
	"math/rand"
)

// StringsConfig supports a list of Strings to publish to a topic. One will be randomly selected at every offset.
type StringsConfig struct {
	Topic   string       `yaml:"topic"`
	Jobs    JobRunnerCfg `yaml:"jobs"`
	Strings []string     `yaml:"strings"`
}

func initStrings(bmux BrokerMux, cfg StringsConfig) {
	if len(cfg.Topic) > 0 && len(cfg.Strings) > 0 {
		NewJobRunner("strings", cfg.Jobs, func() {
			index := 0
			if len(cfg.Strings) > 0 {
				index = rand.Intn(len(cfg.Strings))
			}
			bmux.Publish(cfg.Topic, 0, false, cfg.Strings[index])
		}).Run()
	}
}

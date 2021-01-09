package main

import (
	"math/rand"
)

// StringsConfig supports a list of Strings to publish to a topic. One will be randomly selected at every offset.
type StringsConfig struct {
	Topic   string   `yaml:"topic"`
	Offsets []int    `yaml:"offsets"`
	Strings []string `yaml:"strings"`
}

type jokeJob struct {
	bmux BrokerMux
	cfg  StringsConfig
}

func initStrings(bmux BrokerMux, cfg StringsConfig) {
	if len(cfg.Strings) > 0 && len(cfg.Offsets) > 0 {
		runner := newOffsetJobRunner("strings")
		job := &jokeJob{bmux: bmux, cfg: cfg}
		for _, offset := range cfg.Offsets {
			runner.AddJob(offset, job)
		}
		runner.Run()
	}
}

func (j *jokeJob) Run() {
	// Pick a random joke
	index := 0
	if len(j.cfg.Strings) > 0 {
		index = rand.Intn(len(j.cfg.Strings))
	}

	// Send it
	j.bmux.Publish(j.cfg.Topic, 0, false, j.cfg.Strings[index])
}

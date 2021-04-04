package main

import (
	"math/rand"
	"sort"
	"time"

	log "github.com/sirupsen/logrus"
)

// JobRunnerConfig is a bit of a pain, but in general you can configure raw offsets,
// random intervals, or fixed intervals.
type JobRunnerCfg struct {
	Offsets       []int `yaml:"offsets"`
	RandMin       int   `yaml:"randMin"`
	RandMax       int   `yaml:"randMax"`
	EveryStart    int   `yaml:"everyStart"`
	EveryInterval int   `yaml:"everyInterval"`
}

type JobRunner interface {
	Run()
}

func NewJobRunner(name string, cfg JobRunnerCfg, callback func()) JobRunner {

	if len(cfg.Offsets) > 0 {
		return newOffsetJobRunnerX(name, cfg, callback)
	}

	if cfg.RandMax > 0 {
		return newRandomJobRunnerX(name, cfg, callback)
	}

	if cfg.EveryInterval > 0 {
		return newIntervalJobRunnerX(name, cfg, callback)
	}

	return newOffsetJobRunnerX(name, cfg, callback)
}

//
// Offset jobs
//

type offsetJobRunnerXImpl struct {
	name     string
	callback func()
	offsets  []int
}

func newOffsetJobRunnerX(name string, cfg JobRunnerCfg, callback func()) JobRunner {
	r := &offsetJobRunnerXImpl{name: name, callback: callback, offsets: make([]int, 0, 60)}

	// Copy the offsets from the config
	tmp := make([]int, 0, 60)

	// Vet for sane offsets
	for _, offset := range cfg.Offsets {
		if offset >= 0 && offset < 60 {
			tmp = append(tmp, offset)
		}
	}

	// Sort
	sort.Ints(tmp)

	// Remove duplicates and copy into final array
	lastOffset := -1
	for _, offset := range tmp {
		if offset != lastOffset {
			lastOffset = offset
			r.offsets = append(r.offsets, offset)
		}
	}

	return r
}

func (r *offsetJobRunnerXImpl) Run() {
	// If we have no jobs, bail
	if len(r.offsets) == 0 {
		log.Debugf("job: %s: no work", r.name)
		return
	}

	go r.loop()
}

func (r *offsetJobRunnerXImpl) loop() {
	// Find the next job coming up
	idx := 0
	minute := time.Now().Minute()
	for i, offset := range r.offsets {
		if offset >= minute {
			idx = i
			break
		}
	}

	log.Infof("job: %s: first is %d", r.name, r.offsets[idx])

	// Loop forever, waiting the approriate amount of time for the current entry
	for true {

		// How far off is the current offset?
		delay := r.offsets[idx] - minute
		if delay <= 0 {
			delay = delay + 60
		}

		// Sleep until it is no longer time to sleep.  This adds in some randomness
		// as well since we don't track seconds.  I think we end up +/- 30 seconds?
		log.Debugf("job: offset: %s: sleeping %d minutes", r.name, delay)
		if delay > 0 {
			time.Sleep(time.Duration(delay) * time.Minute)
		}

		// Run the job.  I could confirm we are at the right time, but who cares?
		r.callback()

		// NOTE: All hell breaks loose here if we went past the next job time.  This
		//       also happen often if two jobs have the same offset. Don't do that.
		idx = idx + 1
		if idx >= len(r.offsets) {
			idx = 0
		}

		// Prepare for the next loop
		minute = time.Now().Minute()
	}
}

//
// Intervals, which are just offsets internally
//

func newIntervalJobRunnerX(name string, cfg JobRunnerCfg, callback func()) JobRunner {
	r := &offsetJobRunnerXImpl{name: name, callback: callback, offsets: make([]int, 0, 60)}

	start := cfg.EveryStart
	if start < 0 || start > 59 {
		start = 0
	}

	interval := cfg.EveryInterval
	if start+interval > 60 {
		interval = 0
	}

	for offset := start; offset < 60; {
		r.offsets = append(r.offsets, offset)
		offset = offset + interval
	}

	return r
}

//
// Random jobs
//

type randomJobRunnerXImpl struct {
	name     string
	callback func()
	fixed    int
	random   int
}

func newRandomJobRunnerX(name string, cfg JobRunnerCfg, callback func()) JobRunner {
	fixed := cfg.RandMin
	if fixed < 0 {
		fixed = 0
	}
	random := cfg.RandMax - fixed
	if random < 0 {
		random = 5
	}

	r := &randomJobRunnerXImpl{name: name, callback: callback, fixed: fixed, random: random}
	return r
}

func (r *randomJobRunnerXImpl) Run() {
	go r.loop()
}

func (r *randomJobRunnerXImpl) loop() {
	for true {
		// Delay until the next time
		delay := r.fixed

		if r.random > 0 {
			delay = delay + rand.Intn(r.random)
		}

		if delay <= 0 {
			delay = 2
		}

		log.Debugf("job: random: %s: sleeping %d minutes", r.name, delay)
		time.Sleep(time.Duration(delay) * time.Minute)

		r.callback()
	}
}

package main

import (
	"sort"
	"time"

	log "github.com/sirupsen/logrus"
)

//
// This should all be broken out, but it is fine in here for now.  The general idea is to
// support a list of jobs that want to be run at specific clock offsets (i.e. every X minutes
// after the hour).  It is up to the caller to make sure that these do not overlap, so they
// should be spaced out longer than the actual jobs take.
//
// Note that each one of these is in a goroutine, so intentionally overlapping jobs can
// be handled by having them in different offsetJobRunners.  I'm doing this to allow
// sensors and weather reports overlap, for example, as each type of data source gets
// its own offsetJobRunner.
//

type offsetJob interface {
	Run()
}

type offsetJobRunner interface {
	AddJob(offset int, job offsetJob) bool
	Run()
}

type offsetJobEntry struct {
	offset int
	job    offsetJob
}

type offsetJobRunnerImpl struct {
	name string
	jobs []offsetJobEntry
}

func newOffsetJobRunner(name string) offsetJobRunner {
	r := &offsetJobRunnerImpl{name: name, jobs: make([]offsetJobEntry, 0, 60)}
	log.Printf("jobs: new: %s", name)
	return r
}

func (r *offsetJobRunnerImpl) AddJob(offset int, job offsetJob) bool {
	if offset > 59 || offset < 0 {
		return false
	}

	e := offsetJobEntry{offset: offset, job: job}
	r.jobs = append(r.jobs, e)

	return true
}

func (r *offsetJobRunnerImpl) Run() {
	// If we have no jobs, bail
	if len(r.jobs) == 0 {
		return
	}

	// Sort jobs by offset
	sort.Slice(r.jobs, func(i, j int) bool {
		return r.jobs[i].offset < r.jobs[j].offset
	})

	for _, job := range r.jobs {
		log.Debugf("job: %s: %d", r.name, job.offset)
	}

	// Spin forever
	go r.loop()
}

func (r *offsetJobRunnerImpl) loop() {
	idx := 0

	// Find the next job coming up
	minute := time.Now().Minute()
	for i, job := range r.jobs {
		if job.offset >= minute {
			idx = i
			break
		}
	}

	log.Debugf("job: %s: first is %d", r.name, r.jobs[idx].offset)

	// Loop forever, waiting the approriate amount of time for the current entry
	for true {

		// How far off is the current offset?
		delay := r.jobs[idx].offset - minute
		if delay < 0 {
			delay = delay + 60
		}

		// Sleep until it is no longer time to sleep.  This adds in some randomness
		// as well since we don't track seconds.  I think we end up +/- 30 seconds?
		log.Debugf("job: %s: sleeping %d minutes", r.name, delay)
		if delay > 0 {
			time.Sleep(time.Duration(delay) * time.Minute)
		}

		// Run the job.  I could confirm we are at the right time, but who cares?
		r.jobs[idx].job.Run()

		// NOTE: All hell breaks loose here if we went past the next job time.  This
		//       also happen often if two jobs have the same offset. Don't do that.
		idx = idx + 1
		if idx >= len(r.jobs) {
			idx = 0
		}

		// Prepare for the next loop
		minute = time.Now().Minute()
	}
}
